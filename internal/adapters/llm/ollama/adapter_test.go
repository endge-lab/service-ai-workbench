package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestGenerateStreamsNativeChatResponseWithBearerCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/chat" {
			t.Errorf("path = %q, want /api/chat", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer test-credential" {
			t.Error("bearer credential was not forwarded")
		}
		var payload chatRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if payload.Model != "gpt-oss:20b" || len(payload.Messages) != 2 || payload.Messages[0].Role != "system" {
			t.Fatalf("unexpected chat payload: %#v", payload)
		}
		if len(payload.Format) == 0 || payload.Options == nil || payload.Options.Temperature != 0 {
			t.Fatalf("structured output contract is missing: %#v", payload)
		}
		response.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = response.Write([]byte("{\"message\":{\"role\":\"assistant\",\"content\":\"hello \"},\"done\":false}\n"))
		_, _ = response.Write([]byte("{\"message\":{\"role\":\"assistant\",\"content\":\"world\"},\"done\":true}\n"))
	}))
	defer server.Close()

	adapter := New(Config{
		RequestTimeout:      time.Second,
		MaxResponseBytes:    64 * 1024,
		AllowPrivateNetwork: true,
		AllowInsecureHTTP:   true,
	})
	chunks := make([]string, 0, 2)
	err := adapter.Generate(context.Background(), entities.GenerationRequest{
		ModelRequest: entities.ModelRequest{
			Model:          entities.ModelSnapshot{ProviderModelID: "gpt-oss:20b"},
			SystemPrompt:   "system",
			Messages:       []entities.ModelMessage{{Role: "user", Content: "prompt"}},
			ResponseFormat: entities.FinalAnswerSchema,
		},
		ProviderAccess: entities.ProviderAccess{BaseURL: server.URL, Credential: "test-credential"},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(chunks, "") != "hello world" {
		t.Fatalf("streamed content = %q", strings.Join(chunks, ""))
	}
}

func TestChatEndpointRequiresExplicitInsecureHTTP(t *testing.T) {
	if _, err := chatEndpoint("http://localhost:11434", false); err == nil {
		t.Fatal("insecure HTTP was accepted without the development flag")
	}
	endpoint, err := chatEndpoint("https://ollama.com/api", false)
	if err != nil || endpoint != "https://ollama.com/api/chat" {
		t.Fatalf("endpoint = %q err=%v", endpoint, err)
	}
}

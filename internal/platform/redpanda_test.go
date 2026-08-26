package platform

import (
	"errors"
	"testing"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/config"

	"go.uber.org/zap"
)

func TestNewRedpandaClientDisabled(t *testing.T) {
	client := NewRedpandaClient(&config.Config{}, zap.NewNop())

	if client.Enabled() {
		t.Fatal("expected client to be disabled")
	}

	if _, err := client.NewReader("topic", "group"); !errors.Is(err, ErrRedpandaDisabled) {
		t.Fatalf("expected ErrRedpandaDisabled, got %v", err)
	}

	if _, err := client.NewWriter("topic"); !errors.Is(err, ErrRedpandaDisabled) {
		t.Fatalf("expected ErrRedpandaDisabled, got %v", err)
	}
}

func TestNewRedpandaClientBuildsReaderAndWriter(t *testing.T) {
	client := NewRedpandaClient(&config.Config{
		Redpanda: config.RedpandaConfig{
			Enabled:          true,
			Brokers:          "broker-a:9092, broker-b:9092",
			ClientID:         "service-ai-workbench",
			DialTimeout:      4 * time.Second,
			ReadBatchTimeout: 1500 * time.Millisecond,
			WriteTimeout:     12 * time.Second,
		},
	}, zap.NewNop())

	reader, err := client.NewReader("ai-workbench.commands", "service-ai-workbench")
	if err != nil {
		t.Fatalf("NewReader() transport = %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
	})

	if got := reader.Config().GroupID; got != "service-ai-workbench" {
		t.Fatalf("reader group = %q, want %q", got, "service-ai-workbench")
	}
	if got := reader.Config().Topic; got != "engagement.in-app.commands" {
		t.Fatalf("reader topic = %q, want %q", got, "engagement.in-app.commands")
	}

	writer, err := client.NewWriter("engagement.in-app.commands")
	if err != nil {
		t.Fatalf("NewWriter() transport = %v", err)
	}
	t.Cleanup(func() {
		_ = writer.Close()
	})

	if got := writer.Topic; got != "engagement.in-app.commands" {
		t.Fatalf("writer topic = %q, want %q", got, "engagement.in-app.commands")
	}
	if got := writer.WriteTimeout; got != 12*time.Second {
		t.Fatalf("writer timeout = %s, want %s", got, 12*time.Second)
	}
}

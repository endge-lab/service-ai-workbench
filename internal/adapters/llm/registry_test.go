package llm

import (
	"context"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestRegistryExposesOllamaAndAnthropicAdapters(t *testing.T) {
	registry := NewRegistry(&config.Config{})
	for _, adapter := range []string{"anthropic", "ollama"} {
		_, ok := registry.Resolve(adapter)
		if !ok {
			t.Fatalf("adapter %q is not registered", adapter)
		}
	}

	generator, _ := registry.Resolve("anthropic")
	chunks := make([]string, 0, 4)
	err := generator.Generate(context.Background(), entities.GenerationRequest{
		ModelRequest: entities.ModelRequest{Model: entities.ModelSnapshot{Adapter: "anthropic", DisplayName: "Test"}},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil || len(chunks) == 0 {
		t.Fatalf("anthropic hardcoded adapter failed: chunks=%#v err=%v", chunks, err)
	}
}

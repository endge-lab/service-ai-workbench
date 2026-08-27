package llm

import (
	"context"
	"reflect"
	"testing"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

func TestRegistryExposesBothDeterministicAdapters(t *testing.T) {
	registry := NewRegistry()
	for _, adapter := range []string{"anthropic", "ollama"} {
		generator, ok := registry.Resolve(adapter)
		if !ok {
			t.Fatalf("adapter %q is not registered", adapter)
		}
		first, err := generator.Generate(context.Background(), entities.ModelRequest{Model: entities.ModelSnapshot{Adapter: adapter}})
		if err != nil {
			t.Fatal(err)
		}
		second, err := generator.Generate(context.Background(), entities.ModelRequest{Model: entities.ModelSnapshot{Adapter: adapter}})
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(first, second) || len(first) == 0 {
			t.Fatalf("adapter %q response is not deterministic: %#v / %#v", adapter, first, second)
		}
	}
}

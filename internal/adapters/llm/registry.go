package llm

import (
	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm/anthropic"
	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm/ollama"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type Registry struct {
	adapters map[string]ports.Generator
}

func NewRegistry() *Registry {
	return &Registry{adapters: map[string]ports.Generator{
		"anthropic": anthropic.New(),
		"ollama":    ollama.New(),
	}}
}

func (r *Registry) Resolve(adapter string) (ports.Generator, bool) {
	generator, ok := r.adapters[adapter]
	return generator, ok
}

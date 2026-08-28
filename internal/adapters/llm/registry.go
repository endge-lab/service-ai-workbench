package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm/anthropic"
	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm/ollama"
	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
)

type Registry struct {
	adapters map[string]ports.Generator
}

func NewRegistry(cfg *config.Config) *Registry {
	return &Registry{adapters: map[string]ports.Generator{
		"anthropic": anthropic.New(),
		"ollama": ollama.New(ollama.Config{
			RequestTimeout:      cfg.Ollama.RequestTimeout,
			MaxResponseBytes:    cfg.Ollama.MaxResponseBytes,
			AllowPrivateNetwork: cfg.Ollama.AllowPrivateNetwork,
			AllowInsecureHTTP:   cfg.Ollama.AllowInsecureHTTP,
		}),
	}}
}

func (r *Registry) Resolve(adapter string) (ports.Generator, bool) {
	generator, ok := r.adapters[adapter]
	return generator, ok
}

func (r *Registry) Invoke(ctx context.Context, request ports.StructuredModelRequest) ([]byte, error) {
	generator, ok := r.Resolve(request.Model.Adapter)
	if !ok {
		return nil, fmt.Errorf("unsupported model adapter %q", request.Model.Adapter)
	}
	var output strings.Builder
	err := generator.Generate(ctx, entities.GenerationRequest{
		ModelRequest: entities.ModelRequest{
			Model:          request.Model,
			SystemPrompt:   request.SystemPrompt,
			Messages:       []entities.ModelMessage{{Role: "user", Content: request.UserPrompt}},
			ResponseFormat: request.ResponseFormat,
		},
		ProviderAccess: request.ProviderAccess,
	}, func(chunk string) error {
		output.WriteString(chunk)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []byte(output.String()), nil
}

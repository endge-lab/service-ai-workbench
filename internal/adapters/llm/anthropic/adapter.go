package anthropic

import (
	"context"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Generate(ctx context.Context, request entities.GenerationRequest, emit func(string) error) error {
	chunks := []string{"AI Workbench принял запрос через Anthropic-профиль «", request.ModelRequest.Model.DisplayName, "». ", "Модельный вызов пока отключён; работает тестовый потоковый ответ."}
	for _, chunk := range chunks {
		if err := emit(chunk); err != nil {
			return err
		}
	}
	return nil
}

package ollama

import (
	"context"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Generate(ctx context.Context, prompt string, model entities.ModelSnapshot) ([]string, error) {
	return []string{"AI Workbench принял запрос через Ollama-профиль «", model.DisplayName, "». ", "Модельный вызов пока отключён; работает тестовый потоковый ответ."}, nil
}

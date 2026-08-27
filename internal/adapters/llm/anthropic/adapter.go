package anthropic

import (
	"context"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Generate(ctx context.Context, request entities.ModelRequest) ([]string, error) {
	return []string{"AI Workbench принял запрос через Anthropic-профиль «", request.Model.DisplayName, "». ", "Модельный вызов пока отключён; работает тестовый потоковый ответ."}, nil
}

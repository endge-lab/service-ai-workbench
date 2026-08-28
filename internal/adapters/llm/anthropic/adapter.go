package anthropic

import (
	"context"
	"strings"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Generate(ctx context.Context, request entities.GenerationRequest, emit func(string) error) error {
	system := request.ModelRequest.SystemPrompt
	switch {
	case strings.Contains(system, "ограниченный граф задач"):
		return emit(`{"tasks":[{"id":"task-1","intent":"unsupported","sourceMode":"none","mentions":[],"expectedTypes":[],"requestedAspects":[],"dependsOn":[],"confidence":1,"status":"unsupported"}]}`)
	case strings.Contains(system, "поисковых выражений"):
		return emit(`{"queries":[]}`)
	case strings.Contains(system, "закрытого списка"):
		return emit(`{"selectedCandidateId":"","confidence":0,"requiresClarification":true,"reason":"test adapter"}`)
	case strings.Contains(system, "Классифицируй ответ"):
		return emit(`{"kind":"answer","confidence":1}`)
	case strings.Contains(system, "Исправь структуру"):
		return emit(`{"answer":"Anthropic adapter работает в тестовом режиме.","entityCitations":[],"documentationCitations":[],"limitations":["Реальный вызов Anthropic пока не подключён."]}`)
	case strings.Contains(system, "AI-ассистент платформы Endge"):
		return emit(`{"answer":"Anthropic adapter работает в тестовом режиме.","entityCitations":[],"documentationCitations":[],"limitations":["Реальный вызов Anthropic пока не подключён."]}`)
	}
	chunks := []string{"AI Workbench принял запрос через Anthropic-профиль «", request.ModelRequest.Model.DisplayName, "». ", "Модельный вызов пока отключён; работает тестовый потоковый ответ."}
	for _, chunk := range chunks {
		if err := emit(chunk); err != nil {
			return err
		}
	}
	return nil
}

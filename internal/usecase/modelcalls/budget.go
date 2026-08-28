package modelcalls

import (
	"context"
	"fmt"
	"sync"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type budgetKey struct{}

type Budget struct {
	mutex sync.Mutex
	limit int
	used  int
	usage []entities.PromptUsage
}

func WithBudget(ctx context.Context, limit int) context.Context {
	return context.WithValue(ctx, budgetKey{}, &Budget{limit: limit})
}

func Consume(ctx context.Context) error {
	budget, _ := ctx.Value(budgetKey{}).(*Budget)
	if budget == nil {
		return nil
	}
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	if budget.used >= budget.limit {
		return fmt.Errorf("model call budget exceeded")
	}
	budget.used++
	return nil
}

func Used(ctx context.Context) int {
	budget, _ := ctx.Value(budgetKey{}).(*Budget)
	if budget == nil {
		return 0
	}
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	return budget.used
}

func RecordPromptUsage(ctx context.Context, usage ...entities.PromptUsage) {
	budget, _ := ctx.Value(budgetKey{}).(*Budget)
	if budget == nil || len(usage) == 0 {
		return
	}
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	budget.usage = append(budget.usage, usage...)
}

func PromptUsage(ctx context.Context) []entities.PromptUsage {
	budget, _ := ctx.Value(budgetKey{}).(*Budget)
	if budget == nil {
		return nil
	}
	budget.mutex.Lock()
	defer budget.mutex.Unlock()
	return append([]entities.PromptUsage(nil), budget.usage...)
}

package preparation

import "github.com/endge-lab/service-ai-workbench/internal/domain/entities"

// SourceRouter materializes the already validated per-task source decision.
// It never performs retrieval and cannot broaden a task's source scope.
type SourceRouter struct{}

func (SourceRouter) Route(plan entities.TaskPlan) map[string]entities.SourceMode {
	result := make(map[string]entities.SourceMode, len(plan.Tasks))
	for _, task := range plan.Tasks {
		result[task.ID] = task.SourceMode
	}
	return result
}

func usesDocumentation(mode entities.SourceMode) bool {
	return mode == entities.SourceDocumentation || mode == entities.SourceMixed
}

func usesDomain(mode entities.SourceMode) bool {
	return mode == entities.SourceDomain || mode == entities.SourceMixed
}

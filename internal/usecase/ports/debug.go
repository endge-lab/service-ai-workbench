package ports

import (
	"context"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
)

type RunDebugRecorder interface {
	Record(context.Context, entities.RunDebugRecord)
}

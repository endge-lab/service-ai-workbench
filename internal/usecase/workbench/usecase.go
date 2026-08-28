package workbench

import (
	"errors"
	"time"

	"github.com/endge-lab/service-ai-workbench/internal/domain/entities"
	interactionusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/interaction"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	preparationusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/preparation"
)

var errResponseBufferLimit = errors.New("AI response exceeded the configured buffer limit")

const (
	EventStarted = iota + 1
	EventContentDelta
	EventCompleted
	EventFailed
	EventClarificationRequired
)

type Event struct {
	Type          int
	RunID         string
	MessageID     string
	Delta         string
	ErrorCode     string
	ErrorMessage  string
	InteractionID string
	Clarification *entities.Clarification
	CreatedAt     time.Time
}

type UseCase struct {
	repository               ports.ConversationRepository
	interactions             ports.InteractionRepository
	generators               ports.GeneratorResolver
	interaction              *interactionusecase.Coordinator
	preparation              *preparationusecase.Coordinator
	responseValidator        *preparationusecase.ResponseValidator
	debug                    ports.RunDebugRecorder
	contextMessageLimit      int
	maxPreparationModelCalls int
	responseMaxBytes         int
}

type Dependencies struct {
	Interactions             ports.InteractionRepository
	Interaction              *interactionusecase.Coordinator
	Preparation              *preparationusecase.Coordinator
	ResponseValidator        *preparationusecase.ResponseValidator
	Debug                    ports.RunDebugRecorder
	ContextMessageLimit      int
	MaxPreparationModelCalls int
	ResponseMaxBytes         int
}

func NewUseCase(repository ports.ConversationRepository, generators ports.GeneratorResolver, dependencies ...Dependencies) *UseCase {
	useCase := &UseCase{repository: repository, generators: generators}
	if len(dependencies) > 0 {
		useCase.interactions = dependencies[0].Interactions
		useCase.interaction = dependencies[0].Interaction
		useCase.preparation = dependencies[0].Preparation
		useCase.responseValidator = dependencies[0].ResponseValidator
		useCase.debug = dependencies[0].Debug
		useCase.contextMessageLimit = dependencies[0].ContextMessageLimit
		useCase.maxPreparationModelCalls = dependencies[0].MaxPreparationModelCalls
		useCase.responseMaxBytes = dependencies[0].ResponseMaxBytes
	}
	if useCase.contextMessageLimit < 1 {
		useCase.contextMessageLimit = 10
	}
	if useCase.maxPreparationModelCalls < 0 {
		useCase.maxPreparationModelCalls = 3
	}
	if useCase.responseMaxBytes < 1 {
		useCase.responseMaxBytes = 2 * 1024 * 1024
	}
	return useCase
}

func (u *UseCase) Capabilities() []string { return []string{"anthropic", "ollama"} }

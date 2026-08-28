package bootstrap

import (
	debugfiles "github.com/endge-lab/service-ai-workbench/internal/adapters/debug/files"
	domainsnapshot "github.com/endge-lab/service-ai-workbench/internal/adapters/domain/snapshot"
	knowledgebundle "github.com/endge-lab/service-ai-workbench/internal/adapters/knowledge/bundle"
	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm"
	promptsembedded "github.com/endge-lab/service-ai-workbench/internal/adapters/prompts/embedded"
	grpcv1 "github.com/endge-lab/service-ai-workbench/internal/api/grpc/v1"
	"github.com/endge-lab/service-ai-workbench/internal/auth"
	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/middleware"
	postgresrepo "github.com/endge-lab/service-ai-workbench/internal/repo/postgres"
	interactionusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/interaction"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	preparationusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/preparation"
	workbenchusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/workbench"

	"go.uber.org/fx"
)

func HandlerModules() fx.Option {
	return fx.Options(
		fx.Provide(
			auth.NewResolver,
			fx.Annotate(middleware.NewAuthMiddleware, fx.As(new(middleware.AuthMiddleware))),
			fx.Annotate(postgresrepo.NewWorkbenchRepository, fx.As(new(ports.ConversationRepository)), fx.As(new(ports.InteractionRepository))),
			fx.Annotate(llm.NewRegistry, fx.As(new(ports.GeneratorResolver)), fx.As(new(ports.StructuredModelInvoker))),
			fx.Annotate(promptsembedded.NewCatalog, fx.As(new(ports.PromptCatalog))),
			knowledgebundle.NewRetriever,
			domainsnapshot.NewSelector,
			debugfiles.NewRecorder,
			interactionusecase.NewCoordinator,
			preparationusecase.NewCoordinator,
			preparationusecase.NewResponseValidator,
			newWorkbenchUseCase,
			grpcv1.NewServer,
			newGRPCServer,
		),
	)
}

func newWorkbenchUseCase(
	repository ports.ConversationRepository,
	generators ports.GeneratorResolver,
	interactions ports.InteractionRepository,
	interaction *interactionusecase.Coordinator,
	preparation *preparationusecase.Coordinator,
	responseValidator *preparationusecase.ResponseValidator,
	debug ports.RunDebugRecorder,
	cfg *config.Config,
) *workbenchusecase.UseCase {
	return workbenchusecase.NewUseCase(repository, generators, workbenchusecase.Dependencies{
		Interactions:             interactions,
		Interaction:              interaction,
		Preparation:              preparation,
		ResponseValidator:        responseValidator,
		Debug:                    debug,
		ContextMessageLimit:      cfg.Context.MessageLimit,
		MaxPreparationModelCalls: cfg.Preparation.MaxModelCalls,
		ResponseMaxBytes:         cfg.Preparation.ResponseMaxBytes,
	})
}

package bootstrap

import (
	debugfiles "github.com/endge-lab/service-ai-workbench/internal/adapters/debug/files"
	domainsnapshot "github.com/endge-lab/service-ai-workbench/internal/adapters/domain/snapshot"
	knowledgebundle "github.com/endge-lab/service-ai-workbench/internal/adapters/knowledge/bundle"
	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm"
	grpcv1 "github.com/endge-lab/service-ai-workbench/internal/api/grpc/v1"
	"github.com/endge-lab/service-ai-workbench/internal/auth"
	"github.com/endge-lab/service-ai-workbench/internal/config"
	"github.com/endge-lab/service-ai-workbench/internal/middleware"
	postgresrepo "github.com/endge-lab/service-ai-workbench/internal/repo/postgres"
	"github.com/endge-lab/service-ai-workbench/internal/usecase/ports"
	workbenchusecase "github.com/endge-lab/service-ai-workbench/internal/usecase/workbench"

	"go.uber.org/fx"
)

func HandlerModules() fx.Option {
	return fx.Options(
		fx.Provide(
			auth.NewResolver,
			fx.Annotate(middleware.NewAuthMiddleware, fx.As(new(middleware.AuthMiddleware))),
			fx.Annotate(postgresrepo.NewWorkbenchRepository, fx.As(new(ports.ConversationRepository))),
			fx.Annotate(llm.NewRegistry, fx.As(new(ports.GeneratorResolver))),
			knowledgebundle.NewRetriever,
			domainsnapshot.NewSelector,
			debugfiles.NewRecorder,
			newWorkbenchUseCase,
			grpcv1.NewServer,
			newGRPCServer,
		),
	)
}

func newWorkbenchUseCase(
	repository ports.ConversationRepository,
	generators ports.GeneratorResolver,
	knowledge ports.KnowledgeRetriever,
	domain ports.DomainContextSelector,
	debug ports.RunDebugRecorder,
	cfg *config.Config,
) *workbenchusecase.UseCase {
	return workbenchusecase.NewUseCase(repository, generators, workbenchusecase.Dependencies{
		Knowledge:           knowledge,
		Domain:              domain,
		Debug:               debug,
		ContextMessageLimit: cfg.Context.MessageLimit,
		ContextMaxChars:     cfg.Context.ModelMaxChars,
	})
}

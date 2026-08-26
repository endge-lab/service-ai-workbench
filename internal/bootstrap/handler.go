package bootstrap

import (
	"github.com/endge-lab/service-ai-workbench/internal/adapters/llm"
	grpcv1 "github.com/endge-lab/service-ai-workbench/internal/api/grpc/v1"
	"github.com/endge-lab/service-ai-workbench/internal/auth"
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
			workbenchusecase.NewUseCase,
			grpcv1.NewServer,
			newGRPCServer,
		),
	)
}

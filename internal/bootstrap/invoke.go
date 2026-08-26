package bootstrap

import (
	v1 "github.com/endge-lab/service-ai-workbench/internal/api/http/v1"
	"github.com/endge-lab/service-kit-go/pkg/grpckit"

	"go.uber.org/fx"
)

func InvokeModules() fx.Option {
	return fx.Options(
		fx.Invoke(
			v1.SetupRoutes,
			func(*grpckit.Server) {},
		),
	)
}

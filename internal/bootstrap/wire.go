//go:build wireinject

package bootstrap

import (
	"github.com/google/wire"
)

// Wire set for all providers
var appSet = wire.NewSet(
	provideHTTPClient,
	provideAuthenticator,
	provideAuthService,
	provideLibraryService,
	providePlaybackService,
	providePlaybackServicer,
	provideAppServices,
	provideConfigManager,
	provideCoordinator,
	provideOrchestrator,
	provideKeyMap,
	provideRouter,
	providePageFactory,
	provideAppModel,
	provideApp,
)

// InitializeApp creates and initializes the entire application
// This function will be replaced by generated wire_gen.go
func InitializeApp(opts AppOptions) *App {
	wire.Build(appSet)
	return nil
}

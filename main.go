package main

import (
	"context"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/game_config"
	"github.com/skif48/leaderboard-engine/graceful_shutdown"
	"github.com/skif48/leaderboard-engine/inits"
	"github.com/skif48/leaderboard-engine/logger"
	"github.com/skif48/leaderboard-engine/repositories"
	"github.com/skif48/leaderboard-engine/servers"
	"github.com/skif48/leaderboard-engine/services"
	"go.uber.org/fx"
	"log/slog"
)

func main() {
	var loggerInstance *slog.Logger

	app := fx.New(
		fx.Provide(
			app_config.NewAppConfig,
			logger.InitLogger,
			inits.NewRedisClient,
			repositories.NewUserProfileRepository,
			repositories.NewLeaderboardRepo,
			repositories.NewUserXpRepository,
			services.NewGameActionsService,
			services.NewLeaderboardService,
			game_config.NewGameConfig,
		),
		fx.Populate(&loggerInstance),
		fx.Invoke(servers.RunHttpServer, servers.RunKafkaConsumer),
	)

	if err := app.Err(); err != nil {
		panic(err)
	}

	if err := app.Start(context.Background()); err != nil {
		panic(err)
	}

	graceful_shutdown.WaitForSignals()
}

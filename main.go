package main

import (
	"context"
	"github.com/skif48/leaderboard-engine/repositories"
	"github.com/skif48/leaderboard-engine/servers"
	"github.com/skif48/leaderboard-engine/services"
	"go.uber.org/fx"
)

func main() {
	app := fx.New(
		fx.Provide(
			repositories.NewUserProfileRepository,
			repositories.NewLeaderboardRepo,
			services.NewGameActionsService,
			services.NewGameConfig,
		),
		fx.Invoke(servers.RunHttpServer, servers.RunKafkaConsumer),
	)

	if err := app.Err(); err != nil {
		panic(err)
	}

	if err := app.Start(context.Background()); err != nil {
		panic(err)
	}
	<-app.Done()
}

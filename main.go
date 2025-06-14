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
			services.NewGameActionsService,
		),
		fx.Invoke(servers.RunHttpServer),
	)

	if err := app.Err(); err != nil {
		panic(err)
	}

	if err := app.Start(context.Background()); err != nil {
		panic(err)
	}
}

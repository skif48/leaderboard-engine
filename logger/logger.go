package logger

import (
	"github.com/skif48/leaderboard-engine/app_config"
	"log/slog"
	"os"
)

func InitLogger(ac *app_config.AppConfig) *slog.Logger {
	var level slog.Level
	err := level.UnmarshalText([]byte(ac.LogLevel))
	if err != nil {
		panic(err)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
	slog.SetDefault(logger)
	return logger
}

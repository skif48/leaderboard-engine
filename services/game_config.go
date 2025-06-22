package services

import (
	_ "embed"
	"encoding/json"
)

type GameConfig struct {
	ActionsScoreMap map[string]int `json:"actions_score_map"`
}

//go:embed game_config.json
var gameConfigBytes []byte

func NewGameConfig() *GameConfig {
	gameConfig := &GameConfig{}
	if err := json.Unmarshal(gameConfigBytes, gameConfig); err != nil {
		panic(err)
	}
	return gameConfig
}

package entities

type GameAction struct {
	UserId        string  `json:"user_id"`
	LeaderboardId int     `json:"leaderboard_id"`
	Action        string  `json:"action"`
	Timestamp     float64 `json:"timestamp"`
}

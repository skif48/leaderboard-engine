package entities

type LeaderboardScore struct {
	UserId string `json:"user_id"`
	Score  int    `json:"score"`
}

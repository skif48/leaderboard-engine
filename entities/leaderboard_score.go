package entities

type LeaderboardScore struct {
	Leaderboard int    `json:"leaderboard"`
	UserId      string `json:"user_id"`
	Score       int    `json:"score"`
	Position    int    `json:"position"`
}

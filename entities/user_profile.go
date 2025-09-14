package entities

type UserProfile struct {
	Id          string `json:"id"`
	Nickname    string `json:"nickname"`
	Xp          int    `json:"xp"`
	Level       int    `json:"level"`
	Leaderboard int    `json:"leaderboard"`
	CreatedAt   int64  `json:"createdAt"`
}

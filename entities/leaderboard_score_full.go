package entities

type LeaderboardScoreFull struct {
	LeaderboardScore
	Nickname string `json:"nickname"`
}

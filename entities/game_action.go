package entities

type GameAction struct {
	UserId    string  `json:"user_id"`
	Action    string  `json:"action"`
	Timestamp float64 `json:"timestamp"`
}

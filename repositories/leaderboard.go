package repositories

import (
	"context"
	"fmt"
	"github.com/redis/rueidis"
	"github.com/skif48/leaderboard-engine/entities"
)

type LeaderboardRepo interface {
	AddUser(leaderboard int, userId string) error
	UpdateScore(leaderboard int, userId string, score int) (int, error)
	GetLeaderboard(leaderboard int) ([]*entities.LeaderboardScore, error)
}

type LeaderboardRedisRepo struct {
	c rueidis.Client
}

func NewLeaderboardRepo() LeaderboardRepo {
	client, err := rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{"127.0.0.1:7001"},
		ShuffleInit: true,
	})
	if err != nil {
		panic(err)
	}

	return &LeaderboardRedisRepo{c: client}
}

func (l *LeaderboardRedisRepo) key(leaderboard int) string {
	return fmt.Sprintf("leaderboard:{%d}:data", leaderboard)
}

func (l *LeaderboardRedisRepo) AddUser(leaderboard int, userId string) error {
	return l.c.Do(context.Background(), l.c.B().Zadd().Key(l.key(leaderboard)).ScoreMember().ScoreMember(0, userId).Build()).Error()
}

func (l *LeaderboardRedisRepo) UpdateScore(leaderboard int, userId string, score int) (int, error) {
	updateScoreCmd := l.c.B().Zadd().Key(l.key(leaderboard)).ScoreMember().ScoreMember(float64(score), userId).Build()
	finalScoreCmd := l.c.B().Zscore().Key(l.key(leaderboard)).Member(userId).Build()
	res := l.c.DoMulti(
		context.Background(),
		l.c.B().Multi().Build(),
		updateScoreCmd,
		finalScoreCmd,
		l.c.B().Exec().Build(),
	)
	for _, r := range res {
		if r.Error() != nil {
			return 0, r.Error()
		}
	}
	execResults, err := res[3].AsFloatSlice()
	if err != nil {
		return 0, err
	}

	if len(execResults) < 2 {
		return 0, fmt.Errorf("unexpected number of results from transaction")
	}

	return int(execResults[1]), nil
}

func (l *LeaderboardRedisRepo) GetLeaderboard(leaderboard int) ([]*entities.LeaderboardScore, error) {
	return nil, nil
}

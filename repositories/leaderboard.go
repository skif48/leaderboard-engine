package repositories

import (
	"context"
	"fmt"
	"github.com/redis/rueidis"
	"github.com/skif48/leaderboard-engine/entities"
	"strconv"
)

type LeaderboardRepo interface {
	AddUser(leaderboard int, userId string) error
	UpdateScore(leaderboard int, userId string, score int) (int, error)
	GetLeaderboard(leaderboard int) ([]*entities.LeaderboardScore, error)
	GetAllLeaderboards() (map[int][]*entities.LeaderboardScore, error)
	GetAllLeaderboardsIds() ([]int, error)
	Purge() error
}

type LeaderboardRedisRepo struct {
	c rueidis.Client
}

func NewLeaderboardRepo(c rueidis.Client) LeaderboardRepo {
	return &LeaderboardRedisRepo{c: c}
}

func (l *LeaderboardRedisRepo) key(leaderboard int) string {
	return fmt.Sprintf("leaderboard:{%d}:data", leaderboard)
}

func (l *LeaderboardRedisRepo) updateActiveLeaderboards(leaderboard int) error {
	return l.c.Do(context.Background(), l.c.B().Sadd().Key("leaderboards").Member(strconv.Itoa(leaderboard)).Build()).Error()
}

func (l *LeaderboardRedisRepo) GetAllLeaderboardsIds() ([]int, error) {
	leaderBoards64, err := l.c.Do(context.Background(), l.c.B().Smembers().Key("leaderboards").Build()).AsIntSlice()
	if err != nil {
		return nil, err
	}
	leaderBoards := make([]int, 0, len(leaderBoards64))
	for _, leaderboard := range leaderBoards64 {
		leaderBoards = append(leaderBoards, int(leaderboard))
	}
	return leaderBoards, nil
}

func (l *LeaderboardRedisRepo) GetAllLeaderboards() (map[int][]*entities.LeaderboardScore, error) {
	leaderBoards, err := l.c.Do(context.Background(), l.c.B().Smembers().Key("leaderboards").Build()).AsIntSlice()
	if err != nil {
		return nil, err
	}
	leaderBoardScores := make(map[int][]*entities.LeaderboardScore, len(leaderBoards))
	for _, leaderboard := range leaderBoards {
		scores, err := l.GetLeaderboard(int(leaderboard))
		if err != nil {
			return nil, err
		}
		leaderBoardScores[int(leaderboard)] = scores
	}
	return leaderBoardScores, nil
}

func (l *LeaderboardRedisRepo) AddUser(leaderboard int, userId string) error {
	if err := l.updateActiveLeaderboards(leaderboard); err != nil {
		return err
	}
	return l.c.Do(context.Background(), l.c.B().Zadd().Key(l.key(leaderboard)).ScoreMember().ScoreMember(0, userId).Build()).Error()
}

func (l *LeaderboardRedisRepo) UpdateScore(leaderboard int, userId string, score int) (int, error) {
	updateScoreCmd := l.c.B().Zincrby().Key(l.key(leaderboard)).Increment(float64(score)).Member(userId).Build()
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
	userIds, err := l.c.Do(context.Background(), l.c.B().Zrange().Key(l.key(leaderboard)).Min("0").Max("10").Rev().Build()).AsStrSlice()
	if err != nil {
		return nil, err
	}
	scores := make([]*entities.LeaderboardScore, 0, len(userIds))
	//apparently rueidis doesn't know how to parse zrange rev withscores so had to do this manually
	for position, userId := range userIds {
		score, err := l.c.Do(context.Background(), l.c.B().Zscore().Key(l.key(leaderboard)).Member(userId).Build()).AsFloat64()
		if err != nil {
			return nil, err
		}
		scores = append(scores, &entities.LeaderboardScore{
			Leaderboard: leaderboard,
			UserId:      userId,
			Score:       int(score),
			Position:    position + 1,
		})
	}

	return scores, nil
}

func (l *LeaderboardRedisRepo) Purge() error {
	for _, node := range l.c.Nodes() {
		node.Do(context.Background(), l.c.B().Flushall().Build())
	}
	return nil
}

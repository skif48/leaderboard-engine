package repositories

import (
	"context"
	"fmt"
	"github.com/redis/rueidis"
)

type UserXpRepository interface {
	IncrementXp(userId string, score int) (int, error)
	GetXp(userId string) (int, error)
	GetManyUsersXp(userIds []string) (map[string]int, error)
}

type userXpRepositoryRedis struct {
	c rueidis.Client
}

func NewUserXpRepository(c rueidis.Client) UserXpRepository {
	return &userXpRepositoryRedis{c: c}
}

func (u *userXpRepositoryRedis) key(userId string) string {
	return fmt.Sprintf("user:{%s}:xp", userId)
}

func (u *userXpRepositoryRedis) IncrementXp(userId string, score int) (int, error) {
	xp, err := u.c.Do(context.Background(), u.c.B().Incrby().Key(u.key(userId)).Increment(int64(score)).Build()).ToInt64()
	return int(xp), err
}

func (u *userXpRepositoryRedis) GetXp(userId string) (int, error) {
	xp, err := u.c.Do(context.Background(), u.c.B().Get().Key(u.key(userId)).Build()).ToInt64()
	return int(xp), err
}

func (u *userXpRepositoryRedis) GetManyUsersXp(userIds []string) (map[string]int, error) {
	// todo optimize later for mget command per shard
	xp := make(map[string]int, len(userIds))
	for _, userId := range userIds {
		xp[userId], _ = u.GetXp(userId)
	}
	return xp, nil
}

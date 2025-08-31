package repositories

import (
	"fmt"
	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/qb"
	"github.com/scylladb/gocqlx/v2"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/game_config"
	"time"
)

type UserProfileRepository interface {
	SignUp(r *entities.CreateUserProfileDto) (*entities.UserProfile, error)
	GetManyUserProfiles(userIds []string) ([]*entities.UserProfile, error)
	GetUserProfile(userId string) (*entities.UserProfile, error)
	UpdateXp(userId string, score int) (int, int, bool, error)
	Purge() error
}

type UserProfileRepositoryScylla struct {
	scyllaClient *gocqlx.Session

	gc *game_config.GameConfig
}

func NewUserProfileRepository(ac *app_config.AppConfig, gc *game_config.GameConfig) UserProfileRepository {
	cluster := gocql.NewCluster(ac.ScyllaUrl)
	session, err := gocqlx.WrapSession(cluster.CreateSession())

	if err != nil {
		panic(err)
	}

	err = session.Query("CREATE KEYSPACE IF NOT EXISTS leaderboard WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}", nil).Exec()
	if err != nil {
		panic(err)
	}
	err = session.Query(`CREATE TABLE IF NOT EXISTS leaderboard.user_profile (
    	id uuid,
    	nickname text,
    	xp int,
    	level int,
    	leaderboard int,
    	created_at timestamp,
    	PRIMARY KEY (id))`, nil).Exec()
	if err != nil {
		panic(err)
	}
	return &UserProfileRepositoryScylla{scyllaClient: &session, gc: gc}
}

func (u *UserProfileRepositoryScylla) SignUp(r *entities.CreateUserProfileDto) (*entities.UserProfile, error) {
	id, _ := gocql.RandomUUID()
	createdAt := time.Now()
	q := u.scyllaClient.Query(
		`INSERT INTO leaderboard.user_profile (id,nickname,xp,level,leaderboard,created_at) VALUES (?,?,?,?,?,?)`,
		[]string{":id", ":nickname", ":xp", ":level", ":leaderboard", ":created_at"}).
		BindMap(map[string]interface{}{
			":id":          id,
			":nickname":    r.Nickname,
			":xp":          r.Xp,
			":level":       r.Level,
			":leaderboard": r.Leaderboard,
			":created_at":  createdAt,
		})
	if err := q.ExecRelease(); err != nil {
		return nil, err
	}
	return &entities.UserProfile{
		Id:          id.String(),
		Nickname:    r.Nickname,
		Xp:          r.Xp,
		Level:       r.Level,
		Leaderboard: r.Leaderboard,
		CreatedAt:   createdAt.UnixMilli(),
	}, nil
}

func (u *UserProfileRepositoryScylla) GetManyUserProfiles(userIds []string) ([]*entities.UserProfile, error) {
	uuids := make([]gocql.UUID, len(userIds))
	for i, userIdStr := range userIds {
		uuid, err := gocql.ParseUUID(userIdStr)
		if err != nil {
			return nil, fmt.Errorf("invalid UUID format for user ID %s: %w", userIdStr, err)
		}
		uuids[i] = uuid
	}

	stmt, names := qb.Select("leaderboard.user_profile").Where(qb.In("id")).ToCql()
	q := u.scyllaClient.Query(stmt, names).BindMap(qb.M{"id": uuids})

	var userProfiles []*entities.UserProfile
	if err := q.SelectRelease(&userProfiles); err != nil {
		return nil, err
	}
	return userProfiles, nil
}

func (u *UserProfileRepositoryScylla) GetUserProfile(userId string) (*entities.UserProfile, error) {
	userProfile := &entities.UserProfile{}
	q := u.scyllaClient.Query(`SELECT * FROM leaderboard.user_profile WHERE id = ?`, nil).Bind(userId)
	if err := q.Get(userProfile); err != nil {
		if err == gocql.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	return userProfile, nil
}

func (u *UserProfileRepositoryScylla) UpdateXp(userId string, score int) (int, int, bool, error) {
	for {
		userProfile, err := u.GetUserProfile(userId)
		if err != nil {
			return 0, 0, false, err
		}
		if userProfile == nil {
			return 0, 0, false, fmt.Errorf("user not found during xp update")
		}

		newXp := userProfile.Xp + score
		newLevel := userProfile.Level

		for i, threshold := range u.gc.XpToLevelThresholds {
			if newXp >= threshold && userProfile.Level <= i {
				newLevel = i + 1
			}
		}

		applied := false
		tempUserLevel := 0
		tempUserXp := 0
		updateQuery := u.scyllaClient.Query(`
			UPDATE leaderboard.user_profile
			SET xp = ?, level = ?
			WHERE id = ?
			IF xp = ? AND level = ?`, nil).
			Bind(newXp, newLevel, userId, userProfile.Xp, userProfile.Level)

		if err := updateQuery.Scan(&applied, &tempUserLevel, &tempUserXp); err != nil {
			return 0, 0, false, err
		}

		if applied {
			return newXp, newLevel, newLevel > userProfile.Level, nil
		}
	}
}

func (u *UserProfileRepositoryScylla) Purge() error {
	return u.scyllaClient.Query(`TRUNCATE leaderboard.user_profile`, nil).Exec()
}

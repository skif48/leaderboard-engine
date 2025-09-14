package repositories

import (
	"fmt"
	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/qb"
	"github.com/scylladb/gocqlx/v2"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"time"
)

type UserProfileRepository interface {
	SignUp(r *entities.CreateUserProfileDto) (*entities.UserProfile, error)
	GetManyUserProfiles(userIds []string) ([]*entities.UserProfile, error)
	GetUserProfile(userId string) (*entities.UserProfile, error)
	UpdateLevel(userId string, oldLevel int, newLevel int) (bool, error)
	Purge() error
}

type UserProfileRepositoryScylla struct {
	scyllaClient *gocqlx.Session
}

func NewUserProfileRepository(ac *app_config.AppConfig) UserProfileRepository {
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
    	level int,
    	leaderboard int,
    	created_at timestamp,
    	PRIMARY KEY (id))`, nil).Exec()
	if err != nil {
		panic(err)
	}
	return &UserProfileRepositoryScylla{scyllaClient: &session}
}

func (u *UserProfileRepositoryScylla) SignUp(r *entities.CreateUserProfileDto) (*entities.UserProfile, error) {
	id, _ := gocql.RandomUUID()
	createdAt := time.Now()
	q := u.scyllaClient.Query(
		`INSERT INTO leaderboard.user_profile (id,nickname,level,leaderboard,created_at) VALUES (?,?,?,?,?)`,
		[]string{":id", ":nickname", ":level", ":leaderboard", ":created_at"}).
		BindMap(map[string]interface{}{
			":id":          id,
			":nickname":    r.Nickname,
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

func (u *UserProfileRepositoryScylla) UpdateLevel(userId string, currentLevel int, newLevel int) (bool, error) {
	applied := false
	tempUserLevel := 0
	updateQuery := u.scyllaClient.Query(`
			UPDATE leaderboard.user_profile
			SET level = ?
			WHERE id = ?
			IF level = ?`, nil).
		Bind(newLevel, userId, currentLevel)

	if err := updateQuery.Scan(&applied, &tempUserLevel); err != nil {
		return false, err
	}
	return applied, nil
}

func (u *UserProfileRepositoryScylla) Purge() error {
	return u.scyllaClient.Query(`TRUNCATE leaderboard.user_profile`, nil).Exec()
}

package repositories

import (
	"fmt"
	"github.com/VictoriaMetrics/metrics"
	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/qb"
	"github.com/scylladb/gocqlx/v2"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/graceful_shutdown"
	"time"
)

type UserProfileRepository interface {
	SignUp(r *entities.CreateUserProfileDto) (*entities.UserProfile, error)
	GetManyUserProfiles(userIds []string) ([]*entities.UserProfile, error)
	GetUserProfile(userId string) (*entities.UserProfile, error)
	GetUserProfileEventual(userId string) (*entities.UserProfile, error)
	UpdateLevel(userId string, oldLevel int, newLevel int) (bool, error)
	Purge() error
}

type UserProfileRepositoryScylla struct {
	scyllaClient *gocqlx.Session
}

func trackScyllaLatency(query string) func() {
	start := time.Now()
	return func() {
		metrics.GetOrCreateHistogram(fmt.Sprintf(`scylla_query_latency_milliseconds{query=%q}`, query)).Update(float64(time.Since(start).Milliseconds()))
	}
}

func NewUserProfileRepository(ac *app_config.AppConfig) UserProfileRepository {
	// DDL session — minimal config, no keyspace
	ddlCluster := gocql.NewCluster(ac.ScyllaUrl)
	ddlSession, err := gocqlx.WrapSession(ddlCluster.CreateSession())
	if err != nil {
		panic(err)
	}

	err = ddlSession.Query("CREATE KEYSPACE IF NOT EXISTS leaderboard WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}", nil).Exec()
	if err != nil {
		panic(err)
	}
	err = ddlSession.Query(`CREATE TABLE IF NOT EXISTS leaderboard.user_profile (
    	id uuid,
    	nickname text,
    	level int,
    	leaderboard int,
    	created_at timestamp,
    	PRIMARY KEY (id))`, nil).Exec()
	if err != nil {
		panic(err)
	}
	ddlSession.Close()

	// Main session — tuned for production queries
	cluster := gocql.NewCluster(ac.ScyllaUrl)
	cluster.Keyspace = "leaderboard"
	cluster.NumConns = ac.ScyllaNumConns
	cluster.Consistency = gocql.Quorum
	cluster.PoolConfig.HostSelectionPolicy = gocql.TokenAwareHostPolicy(
		gocql.RoundRobinHostPolicy(),
	)
	session, err := gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		panic(err)
	}
	graceful_shutdown.AddOutputShutdownFunc(func() {
		session.Close()
	})
	return &UserProfileRepositoryScylla{scyllaClient: &session}
}

func (u *UserProfileRepositoryScylla) SignUp(r *entities.CreateUserProfileDto) (*entities.UserProfile, error) {
	defer trackScyllaLatency("sign_up")()
	id, _ := gocql.RandomUUID()
	createdAt := time.Now()
	q := u.scyllaClient.Query(
		`INSERT INTO user_profile (id,nickname,level,leaderboard,created_at) VALUES (?,?,?,?,?)`,
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
	defer trackScyllaLatency("get_many_user_profiles")()
	uuids := make([]gocql.UUID, len(userIds))
	for i, userIdStr := range userIds {
		uuid, err := gocql.ParseUUID(userIdStr)
		if err != nil {
			return nil, fmt.Errorf("invalid UUID format for user ID %s: %w", userIdStr, err)
		}
		uuids[i] = uuid
	}

	stmt, names := qb.Select("user_profile").Where(qb.In("id")).ToCql()
	q := u.scyllaClient.Query(stmt, names).BindMap(qb.M{"id": uuids})

	var userProfiles []*entities.UserProfile
	if err := q.SelectRelease(&userProfiles); err != nil {
		return nil, err
	}
	return userProfiles, nil
}

func (u *UserProfileRepositoryScylla) GetUserProfile(userId string) (*entities.UserProfile, error) {
	defer trackScyllaLatency("get_user_profile")()
	userProfile := &entities.UserProfile{}
	q := u.scyllaClient.Query(`SELECT * FROM user_profile WHERE id = ?`, nil).Bind(userId)
	if err := q.Get(userProfile); err != nil {
		if err == gocql.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	return userProfile, nil
}

func (u *UserProfileRepositoryScylla) GetUserProfileEventual(userId string) (*entities.UserProfile, error) {
	defer trackScyllaLatency("get_user_profile_eventual")()
	userProfile := &entities.UserProfile{}
	q := u.scyllaClient.Query(`SELECT * FROM user_profile WHERE id = ?`, nil).Bind(userId)
	q.Consistency(gocql.One)
	if err := q.Get(userProfile); err != nil {
		if err == gocql.ErrNotFound {
			return nil, nil
		}
		return nil, err
	}

	return userProfile, nil
}

func (u *UserProfileRepositoryScylla) UpdateLevel(userId string, currentLevel int, newLevel int) (bool, error) {
	defer trackScyllaLatency("update_level")()
	applied := false
	tempUserLevel := 0
	updateQuery := u.scyllaClient.Query(`
			UPDATE user_profile
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
	defer trackScyllaLatency("purge")()
	return u.scyllaClient.Query(`TRUNCATE user_profile`, nil).Exec()
}

package repositories

import (
	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v2"
	"github.com/skif48/leaderboard-engine/entities"
	"time"
)

type UserProfileRepository interface {
	SignUp(r *entities.UserProfile) error
	GetUserProfile(userId string) (*entities.UserProfile, error)
}

type UserProfileRepositoryScylla struct {
	scyllaClient *gocqlx.Session
}

func NewUserProfileRepository() UserProfileRepository {
	cluster := gocql.NewCluster("127.0.0.1")
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
    	PRIMARY KEY (id, created_at)) WITH CLUSTERING ORDER BY (created_at DESC)`, nil).Exec()
	if err != nil {
		panic(err)
	}
	return &UserProfileRepositoryScylla{scyllaClient: &session}
}

func (u *UserProfileRepositoryScylla) SignUp(r *entities.UserProfile) error {
	id, _ := gocql.RandomUUID()
	q := u.scyllaClient.Query(
		`INSERT INTO leaderboard.user_profile (id,nickname,xp,level,leaderboard,created_at) VALUES (?,?,?,?,?,?)`,
		[]string{":id", ":nickname", ":xp", ":level", ":leaderboard", ":created_at"}).
		BindMap(map[string]interface{}{
			":id":          id,
			":nickname":    r.Nickname,
			":xp":          r.Xp,
			":level":       r.Level,
			":leaderboard": r.Leaderboard,
			":created_at":  time.Now(),
		})
	return q.Exec()
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

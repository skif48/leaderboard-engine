package services

import (
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/repositories"
)

type LeaderboardService struct {
	leaderboardRepo repositories.LeaderboardRepo
	userProfileRepo repositories.UserProfileRepository
	userXpRepo      repositories.UserXpRepository
}

func NewLeaderboardService(leaderboardRepo repositories.LeaderboardRepo, userProfileRepo repositories.UserProfileRepository, userXpRepo repositories.UserXpRepository) *LeaderboardService {
	return &LeaderboardService{
		leaderboardRepo: leaderboardRepo,
		userProfileRepo: userProfileRepo,
		userXpRepo:      userXpRepo,
	}
}

func (l *LeaderboardService) GetAllLeaderboards() (map[int][]*entities.LeaderboardScoreFull, error) {
	leaderboardIds, err := l.leaderboardRepo.GetAllLeaderboardsIds()
	if err != nil {
		return nil, err
	}
	leaderboardScores := make(map[int][]*entities.LeaderboardScoreFull, len(leaderboardIds))
	for _, leaderboardId := range leaderboardIds {
		scores, err := l.GetLeaderboard(leaderboardId)
		if err != nil {
			return nil, err
		}
		leaderboardScores[leaderboardId] = scores
	}
	return leaderboardScores, nil
}

func (l *LeaderboardService) GetLeaderboard(leaderboardId int) ([]*entities.LeaderboardScoreFull, error) {
	leaderboard, err := l.leaderboardRepo.GetLeaderboard(leaderboardId)
	if err != nil {
		return nil, err
	}
	userIdToScore := make(map[string]int, len(leaderboard))
	userIds := make([]string, 0, len(leaderboard))
	for _, score := range leaderboard {
		userIdToScore[score.UserId] = score.Score
		userIds = append(userIds, score.UserId)
	}
	userProfiles, err := l.userProfileRepo.GetManyUserProfiles(userIds)
	if err != nil {
		return nil, err
	}
	userXps, err := l.userXpRepo.GetManyUsersXp(userIds)

	userIdToProfile := make(map[string]*entities.UserProfile, len(userProfiles))
	for _, profile := range userProfiles {
		profile.Xp = userXps[profile.Id]
		userIdToProfile[profile.Id] = profile
	}

	fullScores := make([]*entities.LeaderboardScoreFull, 0, len(leaderboard))
	for _, score := range leaderboard {
		if profile, exists := userIdToProfile[score.UserId]; exists {
			fullScore := &entities.LeaderboardScoreFull{
				LeaderboardScore: *score,
				Nickname:         profile.Nickname,
			}
			fullScores = append(fullScores, fullScore)
		}
	}
	return fullScores, nil
}

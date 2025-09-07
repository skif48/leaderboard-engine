package servers

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v3"
	"github.com/skif48/leaderboard-engine/app_config"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/repositories"
	"github.com/skif48/leaderboard-engine/services"
	"html/template"
	"log/slog"
	"math/rand/v2"
)

type LeaderboardsPageData struct {
	Leaderboards map[int][]*entities.LeaderboardScoreFull
}

func randRange(min, max int) int {
	return rand.IntN(max-min) + min
}

//go:embed templates/leaderboards.html
var leaderboardsHtmlTemplate string

type HttpHandler struct {
	leaderboardsTemplate *template.Template

	leaderBoardsAmount int

	repo            repositories.UserProfileRepository
	leaderboardRepo repositories.LeaderboardRepo
	gas             *services.GameActionsService
	ls              *services.LeaderboardService
}

func RunHttpServer(ac *app_config.AppConfig, repo repositories.UserProfileRepository, leaderboardRepo repositories.LeaderboardRepo, gas *services.GameActionsService, ls *services.LeaderboardService) {
	leaderboardsTemplate, err := template.New("leaderboards.html").Funcs(template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}).Parse(leaderboardsHtmlTemplate)
	if err != nil {
		panic(err)
	}

	h := &HttpHandler{
		leaderboardsTemplate: leaderboardsTemplate,
		leaderBoardsAmount:   ac.MaxLeaderboards,
		repo:                 repo,
		leaderboardRepo:      leaderboardRepo,
		gas:                  gas,
		ls:                   ls,
	}
	app := fiber.New()

	app.Get("/leaderboards", h.GetLeaderboardsHTML)

	app.Post("/api/v1/users/sign-up", h.SignUp)
	app.Post("/api/v1/users/actions", h.Action)
	app.Get("/api/v1/users/:userId/profile", h.GetUserProfile)

	app.Post("/backoffice-api/purge", h.Purge)

	go func() {
		if err := app.Listen(fmt.Sprintf(":%d", ac.FiberPort)); err != nil {
			panic(err)
		}
	}()
}

func (s *HttpHandler) GetLeaderboardsHTML(c fiber.Ctx) error {
	leaderboards, err := s.ls.GetAllLeaderboards()
	if err != nil {
		slog.Error("Failed to get leaderboards data", "error", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	pageData := LeaderboardsPageData{
		Leaderboards: leaderboards,
	}

	c.Set("Content-Type", "text/html")
	return s.leaderboardsTemplate.Execute(c.Response().BodyWriter(), pageData)
}

func (s *HttpHandler) GetUserProfile(c fiber.Ctx) error {
	userId := c.Params("userId")
	userProfile, err := s.repo.GetUserProfile(userId)
	if err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if userProfile == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	c.Status(fiber.StatusOK)
	return c.JSON(userProfile)
}

func (s *HttpHandler) SignUp(c fiber.Ctx) error {
	req := &entities.SignUpRequest{}
	if err := json.Unmarshal(c.Body(), req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}
	createDto := &entities.CreateUserProfileDto{
		Nickname:    req.Nickname,
		Xp:          0,
		Level:       0,
		Leaderboard: randRange(1, s.leaderBoardsAmount+1),
	}
	userProfile, err := s.repo.SignUp(createDto)
	if err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Status(fiber.StatusCreated)
	return c.JSON(userProfile)
}

func (s *HttpHandler) Action(c fiber.Ctx) error {
	req := &entities.GameAction{}
	if err := json.Unmarshal(c.Body(), req); err != nil {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	userProfile, err := s.repo.GetUserProfile(req.UserId)
	if err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	if userProfile == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}

	req.LeaderboardId = userProfile.Leaderboard

	if err := s.gas.ProduceAction(req); err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusAccepted)
}

func (s *HttpHandler) Purge(c fiber.Ctx) error {
	if err := s.repo.Purge(); err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

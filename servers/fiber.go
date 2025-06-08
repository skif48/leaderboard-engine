package servers

import (
	"encoding/json"
	"github.com/gofiber/fiber/v3"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/repositories"
	"log/slog"
	"math/rand/v2"
)

func randRange(min, max int) int {
	return rand.IntN(max-min) + min
}

type HttpHandler struct {
	repo repositories.UserProfileRepository
}

func RunHttpServer(repo repositories.UserProfileRepository) {
	h := &HttpHandler{
		repo: repo,
	}
	app := fiber.New()

	app.Post("/api/v1/users/sign-up", h.SignUp)
	app.Get("/api/v1/users/:userId/profile", h.GetUserProfile)

	app.Post("/backoffice-api/purge", h.Purge)

	if err := app.Listen(":3000"); err != nil {
		panic(err)
	}
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
		Leaderboard: randRange(0, 1000),
	}
	userProfile, err := s.repo.SignUp(createDto)
	if err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	c.Status(fiber.StatusCreated)
	return c.JSON(userProfile)
}

func (s *HttpHandler) Purge(c fiber.Ctx) error {
	if err := s.repo.Purge(); err != nil {
		slog.Error(err.Error())
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

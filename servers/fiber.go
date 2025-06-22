package servers

import (
	"encoding/json"
	"github.com/gofiber/fiber/v3"
	"github.com/skif48/leaderboard-engine/entities"
	"github.com/skif48/leaderboard-engine/repositories"
	"github.com/skif48/leaderboard-engine/services"
	"log/slog"
	"math/rand/v2"
)

func randRange(min, max int) int {
	return rand.IntN(max-min) + min
}

type HttpHandler struct {
	repo repositories.UserProfileRepository
	gas  *services.GameActionsService
}

func RunHttpServer(repo repositories.UserProfileRepository, gas *services.GameActionsService) {
	h := &HttpHandler{
		repo: repo,
		gas:  gas,
	}
	app := fiber.New()

	app.Post("/api/v1/users/sign-up", h.SignUp)
	app.Post("/api/v1/users/actions", h.Action)
	app.Get("/api/v1/users/:userId/profile", h.GetUserProfile)

	app.Post("/backoffice-api/purge", h.Purge)

	go func() {
		if err := app.Listen(":3000"); err != nil {
			panic(err)
		}
	}()
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
		Leaderboard: randRange(0, 2),
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

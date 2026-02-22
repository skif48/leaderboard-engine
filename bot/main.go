package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTPClient holds the shared HTTP client with connection pooling
var httpClient *http.Client

// Configuration loaded from environment variables
type Config struct {
	BaseURL     string
	RequestRate time.Duration
	ConfigFile  string
	UserCount   int
}

// GameConfig represents the structure of your existing game_config.json
type GameConfig struct {
	ActionsScoreMap     map[string]int `json:"actions_score_map"`
	XpToLevelThresholds []int          `json:"xp_to_level_thresholds"`
}

// SignUpRequest represents the user registration request
type SignUpRequest struct {
	Nickname string `json:"nickname"`
}

// SignUpResponse represents the user registration response
type SignUpResponse struct {
	Id string `json:"id"`
}

// ActionRequest represents the user action request
type ActionRequest struct {
	UserID    string `json:"user_id"`
	Action    string `json:"action"`
	Timestamp int64  `json:"timestamp"`
}

// BotUser represents a single bot user
type BotUser struct {
	ID       string
	Nickname string
}

// NicknameGenerator contains lists of words for generating friendly nicknames
type NicknameGenerator struct {
	adjectives []string
	nouns      []string
	colors     []string
}

// initHTTPClient initializes the shared HTTP client with optimized connection pooling
func initHTTPClient() {
	// Create a custom transport with optimized connection pooling settings
	transport := &http.Transport{
		// Connection pool settings
		MaxIdleConns:        100, // Maximum number of idle connections across all hosts
		MaxIdleConnsPerHost: 100, // Maximum number of idle connections per host
		MaxConnsPerHost:     100, // Maximum number of connections per host (total, not just idle)

		// Keep-alive settings
		IdleConnTimeout:   90 * time.Second, // How long idle connections stay open
		DisableKeepAlives: false,            // Enable keep-alives

		// Connection timeouts
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second, // Connection timeout
			KeepAlive: 30 * time.Second, // Keep-alive probe interval
		}).DialContext,

		// Response timeouts
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,

		// TLS settings
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},

		// Compression
		DisableCompression: false,
	}

	httpClient = &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Overall request timeout
	}

	slog.Info("HTTP client initialized with connection pooling",
		"max_idle_conns", transport.MaxIdleConns,
		"max_idle_conns_per_host", transport.MaxIdleConnsPerHost,
		"max_conns_per_host", transport.MaxConnsPerHost,
		"idle_conn_timeout", transport.IdleConnTimeout.String(),
		"keep_alives_enabled", !transport.DisableKeepAlives)
}

// NewNicknameGenerator creates a new nickname generator with predefined word lists
func NewNicknameGenerator() *NicknameGenerator {
	return &NicknameGenerator{
		adjectives: []string{
			"Happy", "Clever", "Bright", "Swift", "Kind", "Gentle", "Brave", "Calm",
			"Cheerful", "Wise", "Friendly", "Jolly", "Lively", "Merry", "Noble", "Peaceful",
			"Quick", "Smart", "Sunny", "Warm", "Amazing", "Awesome", "Cool", "Epic",
			"Fantastic", "Great", "Mighty", "Super", "Wonderful", "Brilliant", "Creative",
			"Dynamic", "Energetic", "Fantastic", "Graceful", "Humble", "Inspiring", "Joyful",
		},
		nouns: []string{
			"Explorer", "Builder", "Creator", "Dreamer", "Hunter", "Seeker", "Wanderer", "Guardian",
			"Champion", "Hero", "Legend", "Master", "Pioneer", "Sage", "Scholar", "Warrior",
			"Artist", "Inventor", "Navigator", "Pilot", "Runner", "Swimmer", "Climber", "Dancer",
			"Singer", "Writer", "Player", "Gamer", "Coder", "Hacker", "Ninja", "Wizard",
			"Knight", "Ranger", "Scout", "Captain", "Admiral", "General", "Commander", "Leader",
		},
		colors: []string{
			"Blue", "Green", "Red", "Purple", "Orange", "Yellow", "Pink", "Cyan",
			"Silver", "Gold", "Crimson", "Azure", "Emerald", "Violet", "Amber", "Rose",
			"Coral", "Mint", "Lime", "Teal", "Indigo", "Magenta", "Turquoise", "Lavender",
		},
	}
}

// GenerateNickname creates a friendly, human-readable nickname
func (ng *NicknameGenerator) GenerateNickname() string {
	patterns := []func() string{
		// Pattern 1: Adjective + Noun (e.g., "CleverExplorer")
		func() string {
			adj := ng.adjectives[rand.Intn(len(ng.adjectives))]
			noun := ng.nouns[rand.Intn(len(ng.nouns))]
			return adj + noun
		},
		// Pattern 2: Color + Noun (e.g., "BlueWizard")
		func() string {
			color := ng.colors[rand.Intn(len(ng.colors))]
			noun := ng.nouns[rand.Intn(len(ng.nouns))]
			return color + noun
		},
		// Pattern 3: Adjective + Color + Noun (e.g., "BraveBlueKnight")
		func() string {
			adj := ng.adjectives[rand.Intn(len(ng.adjectives))]
			color := ng.colors[rand.Intn(len(ng.colors))]
			noun := ng.nouns[rand.Intn(len(ng.nouns))]
			return adj + color + noun
		},
		// Pattern 4: Noun + Number (e.g., "Explorer42")
		func() string {
			noun := ng.nouns[rand.Intn(len(ng.nouns))]
			number := rand.Intn(100) + 1
			return fmt.Sprintf("%s%d", noun, number)
		},
		// Pattern 5: Adjective + Noun + Number (e.g., "SwiftRunner7")
		func() string {
			adj := ng.adjectives[rand.Intn(len(ng.adjectives))]
			noun := ng.nouns[rand.Intn(len(ng.nouns))]
			number := rand.Intn(100) + 1
			return fmt.Sprintf("%s%s%d", adj, noun, number)
		},
	}

	// Select a random pattern and generate nickname
	pattern := patterns[rand.Intn(len(patterns))]
	return pattern()
}

// GenerateUniqueNickname generates a nickname and ensures it's unique by adding suffix if needed
func (ng *NicknameGenerator) GenerateUniqueNickname(usedNicknames map[string]bool) string {
	maxAttempts := 100

	for attempt := 0; attempt < maxAttempts; attempt++ {
		nickname := ng.GenerateNickname()

		// Check if nickname is already used
		if !usedNicknames[strings.ToLower(nickname)] {
			usedNicknames[strings.ToLower(nickname)] = true
			return nickname
		}
	}

	// If we couldn't generate a unique nickname, add a random suffix
	baseNickname := ng.GenerateNickname()
	suffix := rand.Intn(10000)
	finalNickname := fmt.Sprintf("%s%d", baseNickname, suffix)
	usedNicknames[strings.ToLower(finalNickname)] = true

	return finalNickname
}

// makeHTTPRequest is a helper function that uses the shared HTTP client with context
func makeHTTPRequest(ctx context.Context, method, url string, body io.Reader, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set default content type for POST requests
	if method == "POST" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Add custom headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Set Connection header to keep-alive (though this is default)
	req.Header.Set("Connection", "keep-alive")

	return httpClient.Do(req)
}

func main() {
	// Initialize slog with JSON handler
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize the HTTP client with connection pooling
	initHTTPClient()

	config := loadConfig()
	gameConfig := loadGameConfig(config.ConfigFile)

	// Extract action names from the score map
	actions := extractActionsFromConfig(gameConfig)

	// Register multiple users
	users, err := registerUsers(config.BaseURL, config.UserCount)
	if err != nil {
		slog.Error("Failed to register users", "error", err)
		os.Exit(1)
	}
	slog.Info("Successfully registered users", "count", len(users))

	// Start emitting actions for all users in parallel
	slog.Info("Starting to emit actions for users",
		"user_count", config.UserCount,
		"request_rate", config.RequestRate.String(),
		"available_actions", actions)
	runBots(config.BaseURL, users, actions, config.RequestRate)
}

func loadConfig() Config {
	// Default values
	config := Config{
		BaseURL:     "http://localhost:3000",
		RequestRate: 1 * time.Second,
		ConfigFile:  "game_config.json",
		UserCount:   1,
	}

	// Override with environment variables
	if baseURL := os.Getenv("BOT_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}

	if rateStr := os.Getenv("BOT_REQUEST_RATE_MS"); rateStr != "" {
		if rateMs, err := strconv.Atoi(rateStr); err == nil {
			config.RequestRate = time.Duration(rateMs) * time.Millisecond
		} else {
			slog.Warn("Invalid BOT_REQUEST_RATE_MS value, using default",
				"value", rateStr,
				"default", config.RequestRate.String())
		}
	}

	if configFile := os.Getenv("BOT_GAME_CONFIG_FILE"); configFile != "" {
		config.ConfigFile = configFile
	}

	if userCountStr := os.Getenv("BOT_USER_COUNT"); userCountStr != "" {
		if userCount, err := strconv.Atoi(userCountStr); err == nil && userCount > 0 {
			config.UserCount = userCount
		} else {
			slog.Warn("Invalid BOT_USER_COUNT value, using default",
				"value", userCountStr,
				"default", config.UserCount)
		}
	}

	slog.Info("Configuration loaded",
		"base_url", config.BaseURL,
		"request_rate", config.RequestRate.String(),
		"config_file", config.ConfigFile,
		"user_count", config.UserCount)

	return config
}

func loadGameConfig(filename string) GameConfig {
	file, err := os.Open(filename)
	if err != nil {
		slog.Error("Failed to open game config file", "filename", filename, "error", err)
		os.Exit(1)
	}
	defer file.Close()

	var config GameConfig
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		slog.Error("Failed to decode game config file", "filename", filename, "error", err)
		os.Exit(1)
	}

	if len(config.ActionsScoreMap) == 0 {
		slog.Error("No actions found in game config file", "filename", filename)
		os.Exit(1)
	}

	slog.Info("Loaded game config",
		"filename", filename,
		"action_count", len(config.ActionsScoreMap),
		"xp_thresholds_count", len(config.XpToLevelThresholds))

	// Log the actions and their scores for visibility
	slog.Info("Available actions with scores", "actions_score_map", config.ActionsScoreMap)

	return config
}

func extractActionsFromConfig(config GameConfig) []string {
	actions := make([]string, 0, len(config.ActionsScoreMap))
	for action := range config.ActionsScoreMap {
		actions = append(actions, action)
	}

	slog.Info("Extracted actions from config", "actions", actions)
	return actions
}

func registerUsers(baseURL string, userCount int) ([]BotUser, error) {
	var users []BotUser
	var wg sync.WaitGroup
	var mu sync.Mutex

	errChan := make(chan error, userCount)

	// Initialize nickname generator and tracking
	nicknameGen := NewNicknameGenerator()
	usedNicknames := make(map[string]bool)

	slog.Info("Starting user registration", "user_count", userCount)

	for i := 0; i < userCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Generate a unique, friendly nickname
			mu.Lock()
			nickname := nicknameGen.GenerateUniqueNickname(usedNicknames)
			mu.Unlock()

			userID, err := registerUser(baseURL, nickname)
			if err != nil {
				slog.Error("Failed to register user", "nickname", nickname, "error", err)
				errChan <- fmt.Errorf("failed to register user %s: %w", nickname, err)
				return
			}

			mu.Lock()
			users = append(users, BotUser{
				ID:       userID,
				Nickname: nickname,
			})
			mu.Unlock()

			slog.Info("Successfully registered user", "nickname", nickname, "user_id", userID)
		}(i)
	}

	wg.Wait()
	close(errChan)

	// Check for any registration errors
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return users, nil
}

func registerUser(baseURL, nickname string) (string, error) {
	signUpReq := SignUpRequest{
		Nickname: nickname,
	}

	reqBody, err := json.Marshal(signUpReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal signup request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/users/sign-up", baseURL)

	// Use context with timeout for the request
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := makeHTTPRequest(ctx, "POST", url, bytes.NewBuffer(reqBody), nil)
	if err != nil {
		return "", fmt.Errorf("failed to make signup request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("signup failed with status %d: %s", resp.StatusCode, string(body))
	}

	var signUpResp SignUpResponse
	if err := json.NewDecoder(resp.Body).Decode(&signUpResp); err != nil {
		return "", fmt.Errorf("failed to decode signup response: %w", err)
	}

	return signUpResp.Id, nil
}

func runBots(baseURL string, users []BotUser, actions []string, rate time.Duration) {
	var wg sync.WaitGroup

	// Start a goroutine for each user
	for _, user := range users {
		wg.Add(1)
		go func(u BotUser) {
			defer wg.Done()
			emitActions(baseURL, u.ID, u.Nickname, actions, rate)
		}(user)
	}

	// Wait for all goroutines to complete (they run indefinitely)
	wg.Wait()
}

func emitActions(baseURL, userID, nickname string, actions []string, rate time.Duration) {
	ticker := time.NewTicker(rate)
	defer ticker.Stop()

	actionCount := 0

	slog.Info("Starting action emission for user",
		"nickname", nickname,
		"user_id", userID,
		"rate", rate.String())

	for range ticker.C {
		// Select a random action from available actions
		action := actions[rand.Intn(len(actions))]

		if err := sendAction(baseURL, userID, action); err != nil {
			slog.Error("Failed to send action",
				"nickname", nickname,
				"user_id", userID,
				"action", action,
				"error", err)
		} else {
			actionCount++
			slog.Info("Sent action",
				"nickname", nickname,
				"user_id", userID,
				"action", action,
				"action_count", actionCount)
		}
	}
}

func sendAction(baseURL, userID, action string) error {
	actionReq := ActionRequest{
		UserID:    userID,
		Action:    action,
		Timestamp: time.Now().Unix(),
	}

	reqBody, err := json.Marshal(actionReq)
	if err != nil {
		return fmt.Errorf("failed to marshal action request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/users/actions", baseURL)

	// Use context with timeout for the request
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := makeHTTPRequest(ctx, "POST", url, bytes.NewBuffer(reqBody), nil)
	if err != nil {
		return fmt.Errorf("failed to make action request: %w", err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("action request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

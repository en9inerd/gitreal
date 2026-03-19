package server

import (
	"log/slog"

	"github.com/en9inerd/go-pkgs/middleware"
	"github.com/en9inerd/go-pkgs/router"
	"github.com/en9inerd/gitreal/internal/config"
	"github.com/en9inerd/gitreal/internal/github"
)

func registerRoutes(
	apiGroup *router.Group,
	logger *slog.Logger,
	cfg *config.Config,
	ghClient *github.Client,
) {
	apiGroup.Use(
		middleware.RateLimit(middleware.RateLimitConfig{RPS: 5, Burst: 10}),
		middleware.Logger(logger),
	)
	apiGroup.HandleFunc("POST /score", scoreHandler(logger, cfg, ghClient))
}

func registerWebRoutes(
	webGroup *router.Group,
	logger *slog.Logger,
	cfg *config.Config,
	ghClient *github.Client,
	templates *templateCache,
) {
	webGroup.Use(
		middleware.RateLimit(middleware.RateLimitConfig{RPS: 10, Burst: 20}),
		middleware.Logger(logger),
		middleware.StripSlashes,
	)
	webGroup.HandleFunc("GET /", homePage(logger, templates))
	webGroup.HandleFunc("POST /web/score", scoreWeb(logger, cfg, ghClient, templates))
}

package server

import (
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/en9inerd/gitreal/internal/config"
	"github.com/en9inerd/gitreal/internal/github"
	"github.com/en9inerd/gitreal/ui"
	"github.com/en9inerd/go-pkgs/middleware"
	"github.com/en9inerd/go-pkgs/router"
)

func NewServer(
	logger *slog.Logger,
	cfg *config.Config,
	ghClient *github.Client,
) (http.Handler, error) {
	r := router.New(http.NewServeMux())

	r.Use(
		middleware.Headers(
			"X-Content-Type-Options: nosniff",
			"X-Frame-Options: DENY",
			"Referrer-Policy: no-referrer",
			"Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https://avatars.githubusercontent.com",
			"Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()",
			"Cross-Origin-Opener-Policy: same-origin",
		),
		middleware.RealIP,
		middleware.Recoverer(logger, false),
		middleware.Timeout(55*time.Second),
		middleware.Health,
		middleware.GlobalThrottle(50),
		middleware.SizeLimit(4096),
	)

	templates, err := newTemplateCache()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize templates: %w", err)
	}

	staticFS, err := fs.Sub(ui.Files, "static")
	if err != nil {
		return nil, fmt.Errorf("failed to get static subdirectory: %w", err)
	}
	r.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	if cfg.APIEnabled {
		r.Mount("/api").Route(func(apiGroup *router.Group) {
			registerRoutes(apiGroup, logger, cfg, ghClient)
		})
		logger.Info("API enabled at /api")
	}

	r.Group().Route(func(webGroup *router.Group) {
		registerWebRoutes(webGroup, logger, cfg, ghClient, templates)
	})

	r.NotFoundHandler(notFoundPage(logger, templates))

	return r, nil
}

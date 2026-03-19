package server

import (
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/en9inerd/gitreal/internal/config"
	"github.com/en9inerd/gitreal/internal/github"
	"github.com/en9inerd/gitreal/internal/scorer"
	"github.com/en9inerd/gitreal/ui"
)

type templateData struct {
	CurrentYear int
	PageTitle   string
	PageDesc    string
	Config      *config.Config
	Result      *scorer.ScoreResult
	Error       string
}

type templateCache struct {
	templates map[string]*template.Template
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"scoreClass": func(score, max int) string {
			if max == 0 {
				return "low"
			}
			pct := float64(score) / float64(max) * 100
			switch {
			case pct >= 70:
				return "high"
			case pct >= 40:
				return "mid"
			default:
				return "low"
			}
		},
		"pct": func(score, max int) int {
			if max == 0 {
				return 0
			}
			return int(float64(score) / float64(max) * 100)
		},
	}
}

func newTemplateCache() (*templateCache, error) {
	cache := &templateCache{
		templates: make(map[string]*template.Template),
	}

	tmplFS, err := fs.Sub(ui.Files, "templates")
	if err != nil {
		return nil, fmt.Errorf("failed to get templates subdirectory: %w", err)
	}

	pages, err := fs.Glob(tmplFS, "pages/*.tmpl.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob pages: %w", err)
	}

	for _, page := range pages {
		name := strings.TrimSuffix(filepath.Base(page), ".tmpl.html")
		patterns := []string{
			"layouts/base.tmpl.html",
			"partials/*.tmpl.html",
			page,
		}
		ts, err := template.New("base").Funcs(templateFuncs()).ParseFS(tmplFS, patterns...)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", page, err)
		}
		cache.templates[name] = ts
	}

	partials, err := fs.Glob(tmplFS, "partials/*.tmpl.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob partials: %w", err)
	}

	for _, partial := range partials {
		name := strings.TrimSuffix(filepath.Base(partial), ".tmpl.html")
		ts, err := template.New(name).Funcs(templateFuncs()).ParseFS(tmplFS, partial)
		if err != nil {
			return nil, fmt.Errorf("failed to parse partial %s: %w", partial, err)
		}
		cache.templates[name] = ts
	}

	return cache, nil
}

func (tc *templateCache) render(w http.ResponseWriter, name string, td *templateData) error {
	tmpl, ok := tc.templates[name]
	if !ok {
		return fmt.Errorf("template %s not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, "base", td)
}

func (tc *templateCache) renderFragment(w http.ResponseWriter, name string, td *templateData) error {
	tmpl, ok := tc.templates[name]
	if !ok {
		return fmt.Errorf("template fragment %s not found", name)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	return tmpl.ExecuteTemplate(w, name, td)
}

func renderPage(w http.ResponseWriter, logger *slog.Logger, templates *templateCache, pageName string, td *templateData) {
	if err := templates.render(w, pageName, td); err != nil {
		logger.Error("failed to render page", "page", pageName, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func homePage(logger *slog.Logger, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		renderPage(w, logger, templates, "home", &templateData{
			PageTitle:   "GitReal - GitHub Account Scorer",
			PageDesc:    "Analyze how real a GitHub account is",
			CurrentYear: time.Now().Year(),
		})
	}
}

func notFoundPage(logger *slog.Logger, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		renderPage(w, logger, templates, "404", &templateData{
			PageTitle:   "Not Found - GitReal",
			PageDesc:    "Page not found",
			CurrentYear: time.Now().Year(),
		})
	}
}

func scoreWeb(logger *slog.Logger, cfg *config.Config, ghClient *github.Client, templates *templateCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			logger.Warn("failed to parse form", "error", err)
			renderError(w, templates, "Invalid form data")
			return
		}

		username := strings.TrimSpace(r.FormValue("username"))
		username = strings.TrimPrefix(username, "@")
		if username == "" {
			renderError(w, templates, "GitHub username is required")
			return
		}
		if !github.ValidUsername(username) {
			renderError(w, templates, "Invalid GitHub username format")
			return
		}

		data, err := ghClient.FetchUserData(r.Context(), username)
		if err != nil {
			logger.Warn("failed to fetch user data", "username", username, "error", err)
			if strings.Contains(err.Error(), "not found") {
				renderError(w, templates, fmt.Sprintf("User '%s' not found on GitHub", username))
			} else {
				renderError(w, templates, "Failed to fetch data from GitHub. Try again later.")
			}
			return
		}

		result := scorer.Calculate(data)
		logger.Info("scored user", "username", username, "score", result.Total)

		if err := templates.renderFragment(w, "score_result", &templateData{Result: result}); err != nil {
			logger.Error("failed to render score result", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
	}
}

func renderError(w http.ResponseWriter, templates *templateCache, message string) {
	if err := templates.renderFragment(w, "errors", &templateData{Error: message}); err != nil {
		http.Error(w, message, http.StatusInternalServerError)
	}
}

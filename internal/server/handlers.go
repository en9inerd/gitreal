package server

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/en9inerd/go-pkgs/httpjson"
	"github.com/en9inerd/gitreal/internal/config"
	"github.com/en9inerd/gitreal/internal/github"
	"github.com/en9inerd/gitreal/internal/scorer"
)

func scoreHandler(l *slog.Logger, cfg *config.Config, ghClient *github.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Username string `json:"username"`
		}
		if err := httpjson.DecodeJSON(r, &req); err != nil {
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, err, "invalid request")
			return
		}

		username := strings.TrimPrefix(strings.TrimSpace(req.Username), "@")
		if username == "" {
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("username is required"), "username is required")
			return
		}
		if !github.ValidUsername(username) {
			httpjson.SendErrorJSON(w, r, l, http.StatusBadRequest, errors.New("invalid username format"), "invalid GitHub username")
			return
		}

		data, err := ghClient.FetchUserData(r.Context(), username)
		if err != nil {
			l.Warn("failed to fetch user data", "username", username, "error", err)
			httpjson.SendErrorJSON(w, r, l, http.StatusBadGateway, err, "failed to fetch GitHub data")
			return
		}

		result := scorer.Calculate(data)
		httpjson.WriteJSON(w, result)
		l.Info("scored user", "username", username, "score", result.Total)
	}
}

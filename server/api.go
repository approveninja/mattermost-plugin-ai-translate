package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost/server/public/plugin"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

// initRouter initializes the HTTP router for the plugin.
func (p *Plugin) initRouter() *mux.Router {
	router := mux.NewRouter()

	// Middleware to require that the user is logged in
	router.Use(p.MattermostAuthorizationRequired)

	apiRouter := router.PathPrefix("/api/v1").Subrouter()

	apiRouter.HandleFunc("/hello", p.HelloWorld).Methods(http.MethodGet)
	apiRouter.HandleFunc("/translate", p.handleTranslate).Methods(http.MethodPost)

	return router
}

type translateRequest struct {
	PostID     string `json:"postId"`
	Text       string `json:"text"`
	TargetLang string `json:"targetLang"`
}

func (p *Plugin) handleTranslate(w http.ResponseWriter, r *http.Request) {
	var req translateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	text := req.Text
	if req.PostID != "" && p.resolvePostText != nil {
		resolved, err := p.resolvePostText(req.PostID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Could not load that message.")
			return
		}
		text = resolved
	}
	if strings.TrimSpace(text) == "" {
		writeJSONError(w, http.StatusBadRequest, "Nothing to translate.")
		return
	}

	out, err := p.translator.Translate(r.Context(), text, req.TargetLang)
	if err != nil {
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, translate.ErrNotConfigured), errors.Is(err, translate.ErrReloginRequired), errors.Is(err, translate.ErrQuota):
			status = http.StatusServiceUnavailable
		}
		writeJSONError(w, status, translate.UserMessage(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"translatedText": out})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ServeHTTP demonstrates a plugin that handles HTTP requests by greeting the world.
// The root URL is currently <siteUrl>/plugins/com.mattermost.plugin-starter-template/api/v1/. Replace com.mattermost.plugin-starter-template with the plugin ID.
func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) MattermostAuthorizationRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("Mattermost-User-ID")
		if userID == "" {
			http.Error(w, "Not authorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (p *Plugin) HelloWorld(w http.ResponseWriter, r *http.Request) {
	if _, err := w.Write([]byte("Hello, world!")); err != nil {
		p.API.LogError("Failed to write response", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

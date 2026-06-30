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
	apiRouter.HandleFunc("/auth/status", p.handleAuthStatus).Methods(http.MethodGet)
	apiRouter.HandleFunc("/prefs/lang", p.handleGetLang).Methods(http.MethodGet)
	apiRouter.HandleFunc("/prefs/lang", p.handleSetLang).Methods(http.MethodPut)

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
	userID := r.Header.Get("Mattermost-User-ID")
	text := req.Text
	if req.PostID != "" && p.resolvePostText != nil {
		resolved, err := p.resolvePostText(userID, req.PostID)
		if err != nil {
			writeJSONError(w, http.StatusForbidden, "You don't have access to that message.")
			return
		}
		text = resolved
	}
	if strings.TrimSpace(text) == "" {
		writeJSONError(w, http.StatusBadRequest, "Nothing to translate.")
		return
	}
	if !translate.IsSupported(req.TargetLang) {
		writeJSONError(w, http.StatusBadRequest, "Unsupported language.")
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

// handleGetLang returns the stored language preference for the requesting user.
func (p *Plugin) handleGetLang(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	lang, err := p.prefStore.Get(userID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Could not load preference.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lang": lang})
}

// handleSetLang stores the language preference for the requesting user.
func (p *Plugin) handleSetLang(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	var body struct {
		Lang string `json:"lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request.")
		return
	}
	canonical := strings.ToUpper(strings.TrimSpace(body.Lang))
	if !translate.IsSupported(canonical) {
		writeJSONError(w, http.StatusBadRequest, "Unsupported language.")
		return
	}
	if err := p.prefStore.Set(userID, body.Lang); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Could not save preference.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"lang": canonical})
}

// handleAuthStatus returns whether the Codex auth token is connected.
func (p *Plugin) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	connected := false
	if p.authStatus != nil {
		connected = p.authStatus()
	}
	writeJSON(w, http.StatusOK, map[string]bool{"connected": connected})
}

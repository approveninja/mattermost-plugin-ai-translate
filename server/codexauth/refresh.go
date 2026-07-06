package codexauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

const (
	clientID        = "app_EMoamEEZ73f0CkXaXp7hrann"
	defaultTokenURL = "https://auth.openai.com/oauth/token" //nolint:gosec // public OAuth token endpoint URL, not a secret
)

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// refreshTokens exchanges a refresh token for a fresh access token. The returned
// Tokens always carries a usable refresh token (the rotated one when present,
// else the input). Errors are classified into translate.Err* sentinels.
func refreshTokens(ctx context.Context, httpClient *http.Client, tokenURL, refreshToken string) (Tokens, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return Tokens{}, translate.ErrReloginRequired
	}
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == http.StatusTooManyRequests {
		return Tokens{}, translate.ErrQuota
	}
	if resp.StatusCode != http.StatusOK {
		// OAuth error bodies: {"error":"invalid_grant"} (string) — these mean relogin.
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		switch e.Error {
		case "invalid_grant", "invalid_token", "invalid_request", "refresh_token_reused":
			return Tokens{}, translate.ErrReloginRequired
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return Tokens{}, translate.ErrReloginRequired
		}
		return Tokens{}, fmt.Errorf("%w: refresh status %d: %s", translate.ErrUpstream, resp.StatusCode, string(body))
	}

	var payload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return Tokens{}, fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return Tokens{}, translate.ErrReloginRequired
	}
	rotated := payload.RefreshToken
	if strings.TrimSpace(rotated) == "" {
		rotated = refreshToken
	}
	return Tokens{AccessToken: payload.AccessToken, RefreshToken: rotated, LastRefresh: nowRFC3339()}, nil
}

package codexauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

// Locker provides mutual exclusion across the cluster around token refresh.
type Locker interface {
	Lock(ctx context.Context) (unlock func(), err error)
}

// Authenticator returns valid access tokens, refreshing under a cluster lock.
type Authenticator struct {
	store       *Store
	locker      Locker
	http        *http.Client
	authBaseURL string
	tokenURL    string
}

// NewAuthenticator constructs an Authenticator. authBaseURL defaults to
// "https://auth.openai.com" when empty; the token endpoint is derived as
// authBaseURL + "/oauth/token".
func NewAuthenticator(store *Store, locker Locker, httpClient *http.Client, authBaseURL string) *Authenticator {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if authBaseURL == "" {
		authBaseURL = "https://auth.openai.com"
	}
	authBaseURL = strings.TrimRight(authBaseURL, "/")
	return &Authenticator{
		store:       store,
		locker:      locker,
		http:        httpClient,
		authBaseURL: authBaseURL,
		tokenURL:    authBaseURL + "/oauth/token",
	}
}

// authBase returns the base URL of the authentication server.
func (a *Authenticator) authBase() string { return a.authBaseURL }

const refreshSkew = 5 * time.Minute

// AccessToken returns a non-expired access token, refreshing under a cluster
// lock when the token is expiring within refreshSkew.
func (a *Authenticator) AccessToken(ctx context.Context) (string, error) {
	tokens, ok, err := a.store.Load()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", translate.ErrNotConfigured
	}
	if !accessTokenExpiringWithin(tokens.AccessToken, refreshSkew) {
		return tokens.AccessToken, nil
	}

	unlock, err := a.locker.Lock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()

	// Re-read under lock: another cluster node may have refreshed already.
	tokens, ok, err = a.store.Load()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", translate.ErrNotConfigured
	}
	if !accessTokenExpiringWithin(tokens.AccessToken, refreshSkew) {
		return tokens.AccessToken, nil
	}

	refreshed, err := refreshTokens(ctx, a.http, a.tokenURL, tokens.RefreshToken)
	if err != nil {
		return "", err
	}
	if err := a.store.Save(refreshed); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

// accessTokenExpiringWithin reports whether the JWT access token expires within
// skew. Unparseable tokens or tokens without an exp claim are treated as
// expiring (forcing a refresh attempt).
func accessTokenExpiringWithin(token string, skew time.Duration) bool {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return true
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return true
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp == 0 {
		return true
	}
	return time.Now().Add(skew).After(time.Unix(claims.Exp, 0))
}

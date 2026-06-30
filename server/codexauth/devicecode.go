package codexauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DeviceLogin holds the state needed to display and poll a device-code login.
type DeviceLogin struct {
	UserCode        string
	VerificationURL string
	DeviceAuthID    string
	Interval        int
}

// BeginDeviceLogin requests a user code from the OAuth device endpoint.
func (a *Authenticator) BeginDeviceLogin(ctx context.Context) (DeviceLogin, error) {
	body, _ := json.Marshal(map[string]string{"client_id": clientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.authBase()+"/api/accounts/deviceauth/usercode", bytes.NewReader(body))
	if err != nil {
		return DeviceLogin{}, fmt.Errorf("build device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return DeviceLogin{}, fmt.Errorf("device code request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return DeviceLogin{}, fmt.Errorf("device code request status %d: %s", resp.StatusCode, string(b))
	}
	var d struct {
		UserCode     string `json:"user_code"`
		DeviceAuthID string `json:"device_auth_id"`
		Interval     int    `json:"interval"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return DeviceLogin{}, err
	}
	if d.UserCode == "" || d.DeviceAuthID == "" {
		return DeviceLogin{}, fmt.Errorf("incomplete device code response")
	}
	if d.Interval < 3 {
		d.Interval = 3
	}
	return DeviceLogin{
		UserCode:        d.UserCode,
		VerificationURL: a.authBase() + "/codex/device",
		DeviceAuthID:    d.DeviceAuthID,
		Interval:        d.Interval,
	}, nil
}

// PollDeviceLogin polls once. done=true means tokens were obtained and saved.
func (a *Authenticator) PollDeviceLogin(ctx context.Context, dl DeviceLogin) (bool, error) {
	body, _ := json.Marshal(map[string]string{"device_auth_id": dl.DeviceAuthID, "user_code": dl.UserCode})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.authBase()+"/api/accounts/deviceauth/token", bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("build device poll request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("device poll failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusNotFound:
		return false, nil // still pending
	case http.StatusOK:
		// fallthrough to exchange below
	default:
		b, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("device poll status %d: %s", resp.StatusCode, string(b))
	}

	var d struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeVerifier      string `json:"code_verifier"`
	}
	if err = json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return false, err
	}
	if d.AuthorizationCode == "" || d.CodeVerifier == "" {
		return false, fmt.Errorf("incomplete device token response")
	}

	tokens, err := a.exchangeCode(ctx, d.AuthorizationCode, d.CodeVerifier)
	if err != nil {
		return false, err
	}
	if err := a.store.Save(tokens); err != nil {
		return false, err
	}
	return true, nil
}

func (a *Authenticator) exchangeCode(ctx context.Context, code, verifier string) (Tokens, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {a.authBase() + "/deviceauth/callback"},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, fmt.Errorf("build token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.http.Do(req)
	if err != nil {
		return Tokens{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return Tokens{}, fmt.Errorf("token exchange status %d: %s", resp.StatusCode, string(b))
	}
	var d struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return Tokens{}, err
	}
	if d.AccessToken == "" {
		return Tokens{}, fmt.Errorf("token exchange returned no access_token")
	}
	return Tokens{AccessToken: d.AccessToken, RefreshToken: d.RefreshToken, LastRefresh: nowRFC3339()}, nil
}

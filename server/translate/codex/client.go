package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

// TokenSource yields a currently-valid Codex access token, refreshing as needed.
type TokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// Client calls the Codex (ChatGPT-backend) Responses API to translate text.
type Client struct {
	tokens  TokenSource
	modelFn func() string
	baseURL string
	http    *http.Client
}

// New builds a Codex translation client. baseURL defaults to the ChatGPT codex
// backend when empty; model is a provider func called on every request so that
// config changes take effect without a plugin restart. If modelFn is nil or
// returns "", the model defaults to "gpt-5.5".
func New(tokens TokenSource, model func() string, baseURL string, httpClient *http.Client) *Client {
	if baseURL == "" {
		baseURL = "https://chatgpt.com/backend-api/codex"
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{tokens: tokens, modelFn: model, baseURL: strings.TrimRight(baseURL, "/"), http: httpClient}
}

func (c *Client) Translate(ctx context.Context, text, targetLang string) (string, error) {
	name, ok := translate.LanguageName(targetLang)
	if !ok {
		return "", fmt.Errorf("unsupported target language %q", targetLang)
	}
	token, err := c.tokens.AccessToken(ctx)
	if err != nil {
		return "", err
	}

	model := ""
	if c.modelFn != nil {
		model = c.modelFn()
	}
	if model == "" {
		model = "gpt-5.5"
	}

	payload := map[string]any{
		"model": model,
		"instructions": "You are a translation engine. Translate the user's message into " + name +
			". Output only the translation, preserving Markdown, mentions, emoji, and code spans.",
		"input":  text,
		"store":  false,
		"stream": false,
	}
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/responses", bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", translate.ErrUpstream, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		out, err := extractText(body)
		if err != nil {
			return "", fmt.Errorf("%w: %v", translate.ErrUpstream, err)
		}
		return out, nil
	case http.StatusTooManyRequests:
		return "", translate.ErrQuota
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", translate.ErrReloginRequired
	default:
		return "", fmt.Errorf("%w: status %d: %s", translate.ErrUpstream, resp.StatusCode, string(body))
	}
}

// extractText pulls the translated string from a Responses API body. Adjust to
// the exact shape recorded in Task 0 if it differs.
func extractText(body []byte) (string, error) {
	var parsed struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.OutputText) != "" {
		return parsed.OutputText, nil
	}
	for _, o := range parsed.Output {
		for _, ct := range o.Content {
			if strings.TrimSpace(ct.Text) != "" {
				return ct.Text, nil
			}
		}
	}
	return "", fmt.Errorf("no text in response: %s", string(body))
}

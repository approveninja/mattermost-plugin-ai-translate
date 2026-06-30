package translate

import (
	"context"
	"errors"
)

// Translator turns source text into a target language.
type Translator interface {
	Translate(ctx context.Context, text, targetLang string) (string, error)
}

var (
	// ErrNotConfigured means no Codex credentials are stored yet.
	ErrNotConfigured = errors.New("translation not configured")
	// ErrReloginRequired means the stored OAuth session is invalid/expired.
	ErrReloginRequired = errors.New("codex relogin required")
	// ErrQuota means the provider rate-limited the request (credentials still valid).
	ErrQuota = errors.New("codex quota exhausted")
	// ErrUpstream is any other provider/transport failure.
	ErrUpstream = errors.New("codex upstream error")
)

// UserMessage maps an error to an English, end-user-facing message.
func UserMessage(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "Translation isn't set up yet. An admin must connect Codex with /aitranslate login."
	case errors.Is(err, ErrReloginRequired):
		return "Codex sign-in expired. An admin must reconnect with /aitranslate login."
	case errors.Is(err, ErrQuota):
		return "Translation is rate-limited right now — try again shortly."
	default:
		return "Translation failed, please try again."
	}
}

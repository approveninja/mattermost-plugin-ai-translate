package codexauth

import "github.com/pkg/errors"

const tokensKey = "codex_oauth_tokens"

// Tokens is the persisted Codex OAuth credential bundle.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	LastRefresh  string `json:"last_refresh"`
}

// KV is the subset of pluginapi.Client.KV that codexauth needs.
type KV interface {
	Get(key string, out any) error
	Set(key string, value any) (bool, error)
	Delete(key string) error
}

// Store persists Tokens in the plugin KV store.
type Store struct{ kv KV }

func NewStore(kv KV) *Store { return &Store{kv: kv} }

// Load returns the stored tokens; ok is false when none are stored yet.
func (s *Store) Load() (Tokens, bool, error) {
	var t Tokens
	if err := s.kv.Get(tokensKey, &t); err != nil {
		return Tokens{}, false, errors.Wrap(err, "failed to load codex tokens")
	}
	if t.RefreshToken == "" {
		return Tokens{}, false, nil
	}
	return t, true, nil
}

// Save writes the tokens, overwriting any existing record.
func (s *Store) Save(t Tokens) error {
	if _, err := s.kv.Set(tokensKey, t); err != nil {
		return errors.Wrap(err, "failed to save codex tokens")
	}
	return nil
}

// Clear deletes the stored tokens (logout).
func (s *Store) Clear() error {
	if err := s.kv.Delete(tokensKey); err != nil {
		return errors.Wrap(err, "failed to clear codex tokens")
	}
	return nil
}

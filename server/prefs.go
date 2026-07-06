package main

import (
	"strings"

	"github.com/pkg/errors"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/translate"
)

// kvGetSet is the minimal KV interface needed by langPrefStore.
// Using a local 2-method interface avoids pulling in the Delete method
// from codexauth.KV and lets fakeKV2 in tests satisfy it directly.
type kvGetSet interface {
	Get(key string, out any) error
	Set(key string, value any) (bool, error)
}

// langPrefStore persists per-user target-language preferences in the plugin KV store.
type langPrefStore struct {
	kv       kvGetSet
	fallback func() string
}

func (s *langPrefStore) key(userID string) string { return "lang_pref-" + userID }

// Get returns the stored language for the user. If no preference is set or the
// stored value is no longer supported, it returns the result of fallback().
func (s *langPrefStore) Get(userID string) (string, error) {
	var lang string
	if err := s.kv.Get(s.key(userID), &lang); err != nil {
		return "", errors.Wrap(err, "failed to get language preference")
	}
	if !translate.IsSupported(lang) {
		return s.fallback(), nil
	}
	return strings.ToUpper(lang), nil
}

// Set validates and stores the language preference for the user.
// The language code is uppercased before storage.
// Returns an error if the language is not in the supported set.
func (s *langPrefStore) Set(userID, lang string) error {
	up := strings.ToUpper(strings.TrimSpace(lang))
	if !translate.IsSupported(up) {
		return errors.Errorf("unsupported language %q", lang)
	}
	if _, err := s.kv.Set(s.key(userID), up); err != nil {
		return errors.Wrap(err, "failed to set language preference")
	}
	return nil
}

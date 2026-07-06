package translate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserMessage(t *testing.T) {
	assert.Contains(t, UserMessage(ErrNotConfigured), "isn't set up")
	assert.Contains(t, UserMessage(ErrReloginRequired), "expired")
	assert.Contains(t, UserMessage(ErrQuota), "rate-limited")
	assert.Contains(t, UserMessage(ErrUpstream), "failed")
	assert.Contains(t, UserMessage(errors.New("boom")), "failed")
}

package translate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLanguageName(t *testing.T) {
	name, ok := LanguageName("ES")
	assert.True(t, ok)
	assert.Equal(t, "Spanish", name)

	name, ok = LanguageName("es") // case-insensitive
	assert.True(t, ok)
	assert.Equal(t, "Spanish", name)

	_, ok = LanguageName("XX")
	assert.False(t, ok)
}

func TestIsSupportedAndDefault(t *testing.T) {
	assert.True(t, IsSupported("EN"))
	assert.False(t, IsSupported("XX"))
	assert.Equal(t, "EN", DefaultLanguage)
	assert.Len(t, Languages, 12)
}

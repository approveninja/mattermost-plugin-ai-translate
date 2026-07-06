package codexauth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKV struct{ data map[string][]byte }

func newFakeKV() *fakeKV { return &fakeKV{data: map[string][]byte{}} }

func (f *fakeKV) Get(key string, out any) error {
	b, ok := f.data[key]
	if !ok {
		return nil // pluginapi leaves out at zero value when key missing
	}
	return json.Unmarshal(b, out)
}

func (f *fakeKV) Set(key string, value any) (bool, error) {
	b, _ := json.Marshal(value)
	f.data[key] = b
	return true, nil
}
func (f *fakeKV) Delete(key string) error { delete(f.data, key); return nil }

func TestStoreSaveLoadClear(t *testing.T) {
	s := NewStore(newFakeKV())

	_, ok, err := s.Load()
	require.NoError(t, err)
	assert.False(t, ok)

	require.NoError(t, s.Save(Tokens{AccessToken: "a", RefreshToken: "r", LastRefresh: "t"}))
	got, ok, err := s.Load()
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "a", got.AccessToken)
	assert.Equal(t, "r", got.RefreshToken)

	require.NoError(t, s.Clear())
	_, ok, err = s.Load()
	require.NoError(t, err)
	assert.False(t, ok)
}

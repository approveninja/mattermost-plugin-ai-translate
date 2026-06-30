package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeKV2 struct{ m map[string]string }

func (f *fakeKV2) Get(key string, out any) error {
	if p, ok := out.(*string); ok {
		*p = f.m[key]
	}
	return nil
}
func (f *fakeKV2) Set(key string, value any) (bool, error) {
	f.m[key] = value.(string)
	return true, nil
}

func TestLangPrefStore(t *testing.T) {
	kv := &fakeKV2{m: map[string]string{}}
	s := &langPrefStore{kv: kv, fallback: func() string { return "EN" }}

	got, err := s.Get("u1")
	require.NoError(t, err)
	assert.Equal(t, "EN", got) // fallback

	require.NoError(t, s.Set("u1", "ru"))
	got, err = s.Get("u1")
	require.NoError(t, err)
	assert.Equal(t, "RU", got)

	assert.Error(t, s.Set("u1", "zz")) // unsupported
}

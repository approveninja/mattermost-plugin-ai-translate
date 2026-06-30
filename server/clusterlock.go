package main

import (
	"context"

	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
	"github.com/mattermost/mattermost/server/public/pluginapi/cluster"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/codexauth"
)

// clusterLocker adapts a cluster-wide mutex to codexauth.Locker.
type clusterLocker struct {
	api plugin.API
	key string
}

func (l clusterLocker) Lock(ctx context.Context) (func(), error) {
	m, err := cluster.NewMutex(l.api, l.key)
	if err != nil {
		return nil, err
	}
	if err := m.LockWithContext(ctx); err != nil {
		return nil, err
	}
	return m.Unlock, nil
}

// kvAdapter adapts *pluginapi.KVService (variadic Set) to codexauth.KV.
type kvAdapter struct{ kv *pluginapi.KVService }

func (a kvAdapter) Get(key string, out any) error          { return a.kv.Get(key, out) }
func (a kvAdapter) Set(key string, value any) (bool, error) { return a.kv.Set(key, value) }
func (a kvAdapter) Delete(key string) error                { return a.kv.Delete(key) }

// compile-time interface checks
var _ codexauth.Locker = clusterLocker{}
var _ codexauth.KV = kvAdapter{}

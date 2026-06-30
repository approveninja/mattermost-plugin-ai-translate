package main

import (
	"context"
	"time"

	"github.com/approveninja/mattermost-plugin-ai-translate/server/codexauth"
)

type codexAdmin struct {
	auth  *codexauth.Authenticator
	store *codexauth.Store
}

func (c codexAdmin) Status() bool { _, ok, _ := c.store.Load(); return ok }
func (c codexAdmin) Logout() error { return c.store.Clear() }

func (c codexAdmin) Login(ctx context.Context) (string, string, func() error, error) {
	dl, err := c.auth.BeginDeviceLogin(ctx)
	if err != nil {
		return "", "", nil, err
	}
	wait := func() error {
		deadline := time.Now().Add(15 * time.Minute)
		for time.Now().Before(deadline) {
			time.Sleep(time.Duration(dl.Interval) * time.Second)
			done, err := c.auth.PollDeviceLogin(ctx, dl)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
		return context.DeadlineExceeded
	}
	return dl.UserCode, dl.VerificationURL, wait, nil
}

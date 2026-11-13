package workerctl

import (
	"context"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storeworkers "github.com/FelipeMalacarne/chronicle/internal/store/workers"
)

// RepoStore adapts the workers repository to Store.
type RepoStore struct {
	Repo *storeworkers.Repository
}

// Desired lists every managed account's desired state.
func (s RepoStore) Desired(ctx context.Context) ([]Desired, error) {
	ws, err := s.Repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Desired, 0, len(ws))
	for _, w := range ws {
		out = append(out, Desired{Account: w.AccountID, Running: w.DesiredState == storeworkers.StateRunning, World: w.World, Image: w.Image, AutoLogin: w.AutoLogin, WasLoggedIn: w.WasLoggedIn})
	}
	return out, nil
}

// Session is the account's decrypted game session.
func (s RepoStore) Session(ctx context.Context, account string) (agent.GameSession, error) {
	return s.Repo.Session(ctx, account)
}

// MintPluginToken gives the worker a fresh Chronicle Link token.
func (s RepoStore) MintPluginToken(ctx context.Context, account string) (string, error) {
	return s.Repo.MintPluginToken(ctx, account)
}

// RevokePluginToken revokes the worker's token, if any.
func (s RepoStore) RevokePluginToken(ctx context.Context, account string) error {
	err := s.Repo.RevokePluginToken(ctx, account)
	if err == storeworkers.ErrNotFound {
		return nil
	}
	return err
}

// SetError records (or, with "", clears) the worker's last error.
func (s RepoStore) SetError(ctx context.Context, account, msg string) error {
	err := s.Repo.SetError(ctx, account, msg)
	if err == storeworkers.ErrNotFound {
		return nil
	}
	return err
}

// SetRunningImage moves a running worker to image (a restart on the current
// image); false for a stopped worker or none.
func (s RepoStore) SetRunningImage(ctx context.Context, account, image string) (bool, error) {
	return s.Repo.SetRunningImage(ctx, account, image)
}

// Profile is the account's effective client profile.
func (s RepoStore) Profile(ctx context.Context, account string) (agent.ClientProfile, error) {
	return s.Repo.EffectiveProfile(ctx, account)
}

// RunningPlugins are the plugins the account's client had on at the last
// complete read.
func (s RepoStore) RunningPlugins(ctx context.Context, account string) ([]agent.ProfilePlugin, error) {
	return s.Repo.RunningPlugins(ctx, account)
}

// SetRunningPlugins replaces them.
func (s RepoStore) SetRunningPlugins(ctx context.Context, account string, plugins []agent.ProfilePlugin) error {
	err := s.Repo.SetRunningPlugins(ctx, account, plugins)
	if err == storeworkers.ErrNotFound {
		return nil
	}
	return err
}

// SetLoggedIn records that the client was read logged in.
func (s RepoStore) SetLoggedIn(ctx context.Context, account string, on bool) error {
	err := s.Repo.SetLoggedIn(ctx, account, on)
	if err == storeworkers.ErrNotFound {
		return nil
	}
	return err
}

// RecordLogin records the outcome of a login the reconciler made.
func (s RepoStore) RecordLogin(ctx context.Context, account, result string) error {
	return s.Repo.RecordLogin(ctx, account, result)
}

// OtherClientSince reports another client of the account pushing since then.
func (s RepoStore) OtherClientSince(ctx context.Context, account string, since time.Time) (bool, error) {
	return s.Repo.OtherClientSince(ctx, account, since)
}

// RunningSettings are the option values of the plugins the client had on.
func (s RepoStore) RunningSettings(ctx context.Context, account string) ([]agent.ProfileSetting, error) {
	return s.Repo.RunningSettings(ctx, account)
}

// SetRunningSettings replaces them.
func (s RepoStore) SetRunningSettings(ctx context.Context, account string, settings []agent.ProfileSetting) error {
	err := s.Repo.SetRunningSettings(ctx, account, settings)
	if err == storeworkers.ErrNotFound {
		return nil
	}
	return err
}

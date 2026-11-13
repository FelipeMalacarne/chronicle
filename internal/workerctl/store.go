package workerctl

import (
	"context"

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
		out = append(out, Desired{Account: w.AccountID, Running: w.DesiredState == storeworkers.StateRunning, World: w.World, Image: w.Image})
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

// Profile is the account's effective client profile.
func (s RepoStore) Profile(ctx context.Context, account string) (agent.ClientProfile, error) {
	return s.Repo.EffectiveProfile(ctx, account)
}

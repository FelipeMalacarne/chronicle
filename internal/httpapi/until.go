package httpapi

import (
	"errors"
	"fmt"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	"github.com/FelipeMalacarne/chronicle/internal/core/entities"
)

// A run's wait_until (docs/10 phase 4; docs/08): the run ends when the
// server proves an objective key on the account's facts
// (internal/rununtil). The server writes the account into it, from the
// request's account (accountParam: the session user's own), never taking
// the caller's; and the key must be one the catalog knows.

// errAgentNoUntil: the agent cannot end a run on an objective.
var errAgentNoUntil = errors.New("the agent predates wait_until (run_until): update chronicle on the game machine")

// untilFor fills a run's wait_until with the account whose facts prove it
// and checks the key; a nil until passes.
func (s *Server) untilFor(account string, u *agent.RunUntil) error {
	if u == nil {
		return nil
	}
	if account == "" {
		return errors.New("until: pass ?account=: the server proves the objective on that account's facts")
	}
	k, err := entities.ParseObjectiveKey(u.Objective)
	if err != nil {
		return fmt.Errorf("until: %w", err)
	}
	if s.opts.Catalog != nil {
		if msg := s.opts.Catalog.KnownObjective(k); msg != "" {
			return fmt.Errorf("until: %s", msg)
		}
	}
	u.Account = account
	return nil
}

// runUntilOf is the wait_until of a command's validated params (a run, a
// trigger's run), or nil.
func runUntilOf(p any) *agent.RunUntil {
	switch v := p.(type) {
	case agent.RunParams:
		return v.Until
	case agent.Trigger:
		return v.Run.Until
	}
	return nil
}

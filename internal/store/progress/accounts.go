package progress

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// ErrInvalidAccount rejects a setup with a bad name, mode, guide or layer.
var ErrInvalidAccount = errors.New("invalid account")

// devSubject is the single user of AUTH_MODE=dev.
const devSubject = "dev"

// AccountView is an account as the setup shows it.
type AccountView struct {
	ID      string `json:"id"`
	RSN     string `json:"rsn"`
	Mode    string `json:"mode"`
	GuideID string `json:"guide_id"`
	Layer   string `json:"layer"`
}

// AccountInput is what the setup (V4Setup steps 1 to 3) chooses.
type AccountInput struct {
	RSN     string `json:"rsn"`
	Mode    string `json:"mode"`
	GuideID string `json:"guide_id"`
	Layer   string `json:"layer"`
}

// rsnPattern is the RuneScape display name rule: 1 to 12 letters, digits,
// spaces, hyphens or underscores.
var rsnPattern = regexp.MustCompile(`^[A-Za-z0-9 _-]{1,12}$`)

// NormalizeRSN is the lookup key of a name (lower case, spaces as "_"),
// docs/04 section 6.3.
func NormalizeRSN(rsn string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(rsn)), " ", "_")
}

func accountView(a storedb.Account) AccountView {
	return AccountView{ID: a.ID, RSN: a.Rsn, Mode: a.Mode, GuideID: a.GuideID, Layer: a.Layer}
}

// FindAccount returns the dev user's account with that name, if any.
func (r *Repository) FindAccount(ctx context.Context, rsn string) (AccountView, bool, error) {
	user, err := r.queries.GetUserBySubject(ctx, devSubject)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountView{}, false, nil
	}
	if err != nil {
		return AccountView{}, false, fmt.Errorf("get dev user: %w", err)
	}
	acc, err := r.queries.GetAccountByRSN(ctx, storedb.GetAccountByRSNParams{UserID: user.ID, RsnNormalized: NormalizeRSN(rsn)})
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountView{}, false, nil
	}
	if err != nil {
		return AccountView{}, false, fmt.Errorf("get account: %w", err)
	}
	return accountView(acc), true, nil
}

// CreateAccount creates the account of the setup for the dev user and
// reevaluates it, so the Agora opens on its cursor. A name that already has
// an account returns that account (created false).
func (r *Repository) CreateAccount(ctx context.Context, in AccountInput) (AccountView, bool, error) {
	in.RSN = strings.TrimSpace(in.RSN)
	switch {
	case !rsnPattern.MatchString(in.RSN):
		return AccountView{}, false, fmt.Errorf("%w: name must be 1 to 12 letters, digits, spaces, - or _", ErrInvalidAccount)
	case in.Mode != "ironman" && in.Mode != "hcim" && in.Mode != "uim" && in.Mode != "gim":
		return AccountView{}, false, fmt.Errorf("%w: mode must be ironman, hcim, uim or gim", ErrInvalidAccount)
	case in.Layer != "classic" && in.Layer != "enhanced":
		return AccountView{}, false, fmt.Errorf("%w: layer must be classic or enhanced", ErrInvalidAccount)
	}
	if _, ok := r.content.Guide(in.GuideID); !ok {
		return AccountView{}, false, fmt.Errorf("%w: unknown guide %q", ErrInvalidAccount, in.GuideID)
	}
	if existing, ok, err := r.FindAccount(ctx, in.RSN); err != nil || ok {
		return existing, false, err
	}
	var out AccountView
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		user, err := q.UpsertUserBySubject(ctx, storedb.UpsertUserBySubjectParams{ID: devSubject, Subject: devSubject, Username: devSubject})
		if err != nil {
			return fmt.Errorf("upsert dev user: %w", err)
		}
		normalized := NormalizeRSN(in.RSN)
		acc, err := q.CreateAccount(ctx, storedb.CreateAccountParams{
			ID: "acc_" + normalized, UserID: user.ID, Rsn: in.RSN, RsnNormalized: normalized,
			Mode: in.Mode, GuideID: in.GuideID, Layer: in.Layer,
		})
		if err != nil {
			return fmt.Errorf("create account: %w", err)
		}
		out = accountView(acc)
		_, err = r.Reevaluate(ctx, tx, acc.ID, "")
		return err
	})
	if err != nil {
		return AccountView{}, false, err
	}
	return out, true, nil
}

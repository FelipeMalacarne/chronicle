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

// ErrAccountTaken: the account id (acc_<rsn>) already belongs to another user.
var ErrAccountTaken = errors.New("account belongs to another user")

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

// FindAccount returns the user's account with that name, if any. An empty
// userID means the AUTH_MODE=dev user.
func (r *Repository) FindAccount(ctx context.Context, userID, rsn string) (AccountView, bool, error) {
	if userID == "" {
		user, err := r.queries.GetUserBySubject(ctx, devSubject)
		if errors.Is(err, pgx.ErrNoRows) {
			return AccountView{}, false, nil
		}
		if err != nil {
			return AccountView{}, false, fmt.Errorf("get dev user: %w", err)
		}
		userID = user.ID
	}
	acc, err := r.queries.GetAccountByRSN(ctx, storedb.GetAccountByRSNParams{UserID: userID, RsnNormalized: NormalizeRSN(rsn)})
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountView{}, false, nil
	}
	if err != nil {
		return AccountView{}, false, fmt.Errorf("get account: %w", err)
	}
	return accountView(acc), true, nil
}

// CreateAccount creates the account of the setup for userID (empty: the dev
// user, created on demand) and reevaluates it, so the Agora opens on its
// cursor. A name that already has an account of this user returns that
// account (created false); one that belongs to another user is
// ErrAccountTaken, since account ids are acc_<rsn>.
func (r *Repository) CreateAccount(ctx context.Context, userID string, in AccountInput) (AccountView, bool, error) {
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
	if existing, ok, err := r.FindAccount(ctx, userID, in.RSN); err != nil || ok {
		return existing, false, err
	}
	normalized := NormalizeRSN(in.RSN)
	switch _, err := r.queries.GetAccount(ctx, "acc_"+normalized); {
	case err == nil:
		return AccountView{}, false, ErrAccountTaken
	case !errors.Is(err, pgx.ErrNoRows):
		return AccountView{}, false, fmt.Errorf("get account: %w", err)
	}
	var out AccountView
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		q := r.queries.WithTx(tx)
		owner := userID
		if owner == "" {
			user, err := q.UpsertUserBySubject(ctx, storedb.UpsertUserBySubjectParams{ID: devSubject, Subject: devSubject, Username: devSubject})
			if err != nil {
				return fmt.Errorf("upsert dev user: %w", err)
			}
			owner = user.ID
		}
		acc, err := q.CreateAccount(ctx, storedb.CreateAccountParams{
			ID: "acc_" + normalized, UserID: owner, Rsn: in.RSN, RsnNormalized: normalized,
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

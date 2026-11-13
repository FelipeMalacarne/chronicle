package workers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/FelipeMalacarne/chronicle/internal/agent"
	storedb "github.com/FelipeMalacarne/chronicle/internal/store/db"
)

// Client profiles (docs/11 section 4.3, phase F5; migration 0013): what a
// worker's client runs and how it is configured. A profile belongs to a
// user and may build on a model (one level); an account names one profile
// and may add its own settings. The effective profile goes to the worker in
// its bootstrap (agent.BootstrapParams.Profile).

// Errors of the client profiles.
var (
	// ErrProfileNotFound: no profile (or model) of that name for the user.
	ErrProfileNotFound = errors.New("workers: client profile not found")
	// ErrProfileInUse: the profile is the model of another profile or the
	// profile of an account.
	ErrProfileInUse = errors.New("workers: the client profile is in use")
)

var reProfileName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

// Profile is a stored client profile: its own plugins and settings (not
// merged with its model), the model's name and the accounts that use it.
type Profile struct {
	Name      string                 `json:"name"`
	Parent    string                 `json:"parent,omitempty"`
	Plugins   []agent.ProfilePlugin  `json:"plugins"`
	Settings  []agent.ProfileSetting `json:"settings"`
	Accounts  []string               `json:"accounts"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// AccountClient is an account's client profile: the profile it names, its
// own settings over it, and what the worker gets from both.
type AccountClient struct {
	AccountID string                 `json:"account_id"`
	Profile   string                 `json:"profile,omitempty"`
	Settings  []agent.ProfileSetting `json:"settings"`
	Effective agent.ClientProfile    `json:"effective"`
}

// ListProfiles returns the user's profiles by name.
func (r *Repository) ListProfiles(ctx context.Context, userID string) ([]Profile, error) {
	rows, err := r.queries.ListClientProfiles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list profiles: %w", err)
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		names[row.ID] = row.Name
	}
	out := make([]Profile, 0, len(rows))
	for _, row := range rows {
		p, err := profileOf(ctx, r.queries, row, names[row.ParentID.String])
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// GetProfile returns one of the user's profiles.
func (r *Repository) GetProfile(ctx context.Context, userID, name string) (Profile, error) {
	row, err := profileByName(ctx, r.queries, userID, name)
	if err != nil {
		return Profile{}, err
	}
	return profileOf(ctx, r.queries, row, "")
}

// PutProfile creates or replaces the user's profile name: its model (""
// for none) and its plugins and settings, all at once.
func (r *Repository) PutProfile(ctx context.Context, userID, name, parent string, layer agent.ClientProfile) (Profile, error) {
	if !reProfileName.MatchString(name) {
		return Profile{}, fmt.Errorf("%w: a profile name is 1 to 48 lowercase letters, digits, '-' or '_'", ErrInvalid)
	}
	if parent == name {
		return Profile{}, fmt.Errorf("%w: a profile cannot be its own model", ErrInvalid)
	}
	if err := agent.ValidateProfile(layer); err != nil {
		return Profile{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	var out Profile
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		if lockErr := lockUser(ctx, q, userID); lockErr != nil {
			return lockErr
		}
		existing, readErr := q.GetClientProfileByName(ctx, storedb.GetClientProfileByNameParams{UserID: userID, Name: name})
		found := readErr == nil
		if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
			return fmt.Errorf("read profile: %w", readErr)
		}
		parentID, modelErr := modelID(ctx, q, userID, name, parent, existing.ID, found)
		if modelErr != nil {
			return modelErr
		}
		id := existing.ID
		var writeErr error
		if found {
			writeErr = q.SetClientProfileParent(ctx, storedb.SetClientProfileParentParams{ID: id, ParentID: parentID})
		} else {
			id = newID("prf_")
			writeErr = q.InsertClientProfile(ctx, storedb.InsertClientProfileParams{ID: id, UserID: userID, Name: name, ParentID: parentID})
		}
		if writeErr != nil {
			return fmt.Errorf("write profile: %w", writeErr)
		}
		if layerErr := writeLayer(ctx, q, id, layer); layerErr != nil {
			return layerErr
		}
		row, readErr := q.GetClientProfile(ctx, id)
		if readErr != nil {
			return fmt.Errorf("read profile: %w", readErr)
		}
		out, readErr = profileOf(ctx, q, row, parent)
		return readErr
	})
	return out, err
}

// modelID checks parent as the model of the profile name (id, when found)
// and returns the model's id; no parent is no model. One level: the model
// has no model, and a profile that is a model gets none.
func modelID(ctx context.Context, q *storedb.Queries, userID, name, parent, id string, found bool) (pgtype.Text, error) {
	if parent == "" {
		return pgtype.Text{}, nil
	}
	model, err := profileByName(ctx, q, userID, parent)
	if err != nil {
		return pgtype.Text{}, fmt.Errorf("model %q: %w", parent, err)
	}
	if model.ParentID.Valid {
		return pgtype.Text{}, fmt.Errorf("%w: %q has a model of its own, so it cannot be a model (one level)", ErrInvalid, parent)
	}
	if found {
		children, countErr := q.CountClientProfileChildren(ctx, pgtype.Text{String: id, Valid: true})
		if countErr != nil {
			return pgtype.Text{}, fmt.Errorf("count profiles on the model: %w", countErr)
		}
		if children > 0 {
			return pgtype.Text{}, fmt.Errorf("%w: %q is the model of %d profiles, so it cannot have a model (one level)", ErrInvalid, name, children)
		}
	}
	return pgtype.Text{String: model.ID, Valid: true}, nil
}

// DeleteProfile deletes the user's profile, unless it is a model or an
// account's profile.
func (r *Repository) DeleteProfile(ctx context.Context, userID, name string) error {
	return r.inTx(ctx, func(q *storedb.Queries) error {
		if err := lockUser(ctx, q, userID); err != nil {
			return err
		}
		row, err := profileByName(ctx, q, userID, name)
		if err != nil {
			return err
		}
		children, err := q.CountClientProfileChildren(ctx, pgtype.Text{String: row.ID, Valid: true})
		if err != nil {
			return fmt.Errorf("count profiles on the model: %w", err)
		}
		if children > 0 {
			return fmt.Errorf("%w: %q is the model of %d profiles", ErrProfileInUse, name, children)
		}
		accounts, err := q.ListClientProfileAccounts(ctx, pgtype.Text{String: row.ID, Valid: true})
		if err != nil {
			return fmt.Errorf("list the profile's accounts: %w", err)
		}
		if len(accounts) > 0 {
			return fmt.Errorf("%w: %q is the profile of %v", ErrProfileInUse, name, accounts)
		}
		if _, err = q.DeleteClientProfile(ctx, row.ID); err != nil {
			return fmt.Errorf("delete profile: %w", inUse(err))
		}
		return nil
	})
}

// AccountClient returns the account's client profile (an empty one when it
// names none).
func (r *Repository) AccountClient(ctx context.Context, account string) (AccountClient, error) {
	if _, err := r.queries.GetAccount(ctx, account); errors.Is(err, pgx.ErrNoRows) {
		return AccountClient{}, ErrNotFound
	} else if err != nil {
		return AccountClient{}, fmt.Errorf("read account: %w", err)
	}
	return accountClient(ctx, r.queries, account)
}

// SetAccountClient sets the account's profile (one of the account's user's;
// "" for none) and replaces the account's own settings.
func (r *Repository) SetAccountClient(ctx context.Context, account, profile string, settings []agent.ProfileSetting) (AccountClient, error) {
	if err := agent.ValidateProfile(agent.ClientProfile{Settings: settings}); err != nil {
		return AccountClient{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	var out AccountClient
	err := r.inTx(ctx, func(q *storedb.Queries) error {
		acc, txErr := lockAccount(ctx, q, account)
		if txErr != nil {
			return txErr
		}
		var profileID pgtype.Text
		if profile != "" {
			row, readErr := profileByName(ctx, q, acc.UserID, profile)
			if readErr != nil {
				return readErr
			}
			profileID = pgtype.Text{String: row.ID, Valid: true}
		}
		if txErr = q.UpsertAccountClient(ctx, storedb.UpsertAccountClientParams{AccountID: account, ProfileID: profileID}); txErr != nil {
			return fmt.Errorf("set the account's profile: %w", txErr)
		}
		if txErr = q.DeleteAccountClientSettings(ctx, account); txErr != nil {
			return fmt.Errorf("clear the account's settings: %w", txErr)
		}
		for _, s := range settings {
			if txErr = q.InsertAccountClientSetting(ctx, storedb.InsertAccountClientSettingParams{
				AccountID: account, ConfigGroup: s.Group, ConfigKey: s.Key, Value: s.Value,
			}); txErr != nil {
				return fmt.Errorf("write the account's settings: %w", txErr)
			}
		}
		out, txErr = accountClient(ctx, q, account)
		return txErr
	})
	return out, err
}

// EffectiveProfile is what the account's worker applies: the model, the
// profile and the account's own settings, merged.
func (r *Repository) EffectiveProfile(ctx context.Context, account string) (agent.ClientProfile, error) {
	ac, err := accountClient(ctx, r.queries, account)
	return ac.Effective, err
}

func accountClient(ctx context.Context, q *storedb.Queries, account string) (AccountClient, error) {
	out := AccountClient{AccountID: account, Settings: []agent.ProfileSetting{}}
	var layers []agent.ClientProfile
	ac, err := q.GetAccountClient(ctx, account)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return out, fmt.Errorf("read the account's profile: %w", err)
	case ac.ProfileID.Valid:
		profile, readErr := profileLayers(ctx, q, ac.ProfileID.String)
		if readErr != nil {
			return out, readErr
		}
		out.Profile, layers = profile.name, profile.layers
	}
	rows, err := q.ListAccountClientSettings(ctx, account)
	if err != nil {
		return out, fmt.Errorf("read the account's settings: %w", err)
	}
	for _, s := range rows {
		out.Settings = append(out.Settings, agent.ProfileSetting{Group: s.ConfigGroup, Key: s.ConfigKey, Value: s.Value})
	}
	out.Effective = agent.MergeProfiles(append(layers, agent.ClientProfile{Settings: out.Settings})...)
	return out, nil
}

// profileStack is a profile's name and its layers: the model's, if any,
// then its own.
type profileStack struct {
	name   string
	layers []agent.ClientProfile
}

func profileLayers(ctx context.Context, q *storedb.Queries, profileID string) (profileStack, error) {
	var out profileStack
	row, err := q.GetClientProfile(ctx, profileID)
	if err != nil {
		return out, fmt.Errorf("read the account's profile: %w", err)
	}
	out.name = row.Name
	if row.ParentID.Valid {
		model, modelErr := readLayer(ctx, q, row.ParentID.String)
		if modelErr != nil {
			return out, modelErr
		}
		out.layers = append(out.layers, model)
	}
	own, err := readLayer(ctx, q, row.ID)
	if err != nil {
		return out, err
	}
	out.layers = append(out.layers, own)
	return out, nil
}

// readLayer is one profile's own plugins and settings.
func readLayer(ctx context.Context, q *storedb.Queries, profileID string) (agent.ClientProfile, error) {
	out := agent.ClientProfile{Plugins: []agent.ProfilePlugin{}, Settings: []agent.ProfileSetting{}}
	plugins, err := q.ListClientProfilePlugins(ctx, profileID)
	if err != nil {
		return out, fmt.Errorf("read the profile's plugins: %w", err)
	}
	for _, p := range plugins {
		out.Plugins = append(out.Plugins, agent.ProfilePlugin{
			Source: p.Source, Name: p.Name, Version: p.Version, ClassName: p.ClassName, Start: p.StartOnBoot,
		})
	}
	settings, err := q.ListClientProfileSettings(ctx, profileID)
	if err != nil {
		return out, fmt.Errorf("read the profile's settings: %w", err)
	}
	for _, s := range settings {
		out.Settings = append(out.Settings, agent.ProfileSetting{Group: s.ConfigGroup, Key: s.ConfigKey, Value: s.Value})
	}
	return out, nil
}

// writeLayer replaces a profile's plugins (in their order) and settings.
func writeLayer(ctx context.Context, q *storedb.Queries, profileID string, layer agent.ClientProfile) error {
	if err := q.DeleteClientProfilePlugins(ctx, profileID); err != nil {
		return fmt.Errorf("clear the profile's plugins: %w", err)
	}
	for i, p := range layer.Plugins {
		if err := q.InsertClientProfilePlugin(ctx, storedb.InsertClientProfilePluginParams{
			ProfileID: profileID, Position: int32(i), Source: p.Source, Name: p.Name, //nolint:gosec // at most agent's 100 plugins
			Version: p.Version, ClassName: p.ClassName, StartOnBoot: p.Start,
		}); err != nil {
			return fmt.Errorf("write the profile's plugins: %w", err)
		}
	}
	if err := q.DeleteClientProfileSettings(ctx, profileID); err != nil {
		return fmt.Errorf("clear the profile's settings: %w", err)
	}
	for _, s := range layer.Settings {
		if err := q.InsertClientProfileSetting(ctx, storedb.InsertClientProfileSettingParams{
			ProfileID: profileID, ConfigGroup: s.Group, ConfigKey: s.Key, Value: s.Value,
		}); err != nil {
			return fmt.Errorf("write the profile's settings: %w", err)
		}
	}
	return nil
}

// profileOf is a profile row with its plugins, settings and accounts;
// parent is the model's name when the caller has it.
func profileOf(ctx context.Context, q *storedb.Queries, row storedb.ClientProfile, parent string) (Profile, error) {
	layer, err := readLayer(ctx, q, row.ID)
	if err != nil {
		return Profile{}, err
	}
	if row.ParentID.Valid && parent == "" {
		model, modelErr := q.GetClientProfile(ctx, row.ParentID.String)
		if modelErr != nil {
			return Profile{}, fmt.Errorf("read the model: %w", modelErr)
		}
		parent = model.Name
	}
	accounts, err := q.ListClientProfileAccounts(ctx, pgtype.Text{String: row.ID, Valid: true})
	if err != nil {
		return Profile{}, fmt.Errorf("list the profile's accounts: %w", err)
	}
	return Profile{
		Name: row.Name, Parent: parent, Plugins: layer.Plugins, Settings: layer.Settings,
		Accounts: accounts, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func profileByName(ctx context.Context, q *storedb.Queries, userID, name string) (storedb.ClientProfile, error) {
	row, err := q.GetClientProfileByName(ctx, storedb.GetClientProfileByNameParams{UserID: userID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, fmt.Errorf("%w: %q", ErrProfileNotFound, name)
	}
	if err != nil {
		return row, fmt.Errorf("read profile: %w", err)
	}
	return row, nil
}

// lockUser locks the users row: every profile write takes it first.
func lockUser(ctx context.Context, q *storedb.Queries, userID string) error {
	_, err := q.LockUserForProfiles(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock user: %w", err)
	}
	return nil
}

// inUse maps a foreign key violation (a profile still referenced) to
// ErrProfileInUse.
func inUse(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23503" {
		return ErrProfileInUse
	}
	return err
}

func newID(prefix string) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

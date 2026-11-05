package ingest

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	coreingest "github.com/FelipeMalacarne/ironman-planner/internal/core/ingest"
	storedb "github.com/FelipeMalacarne/ironman-planner/internal/store/db"
)

// CreatedToken is returned once by the dev-only token bootstrap endpoint.
type CreatedToken struct {
	ID        string                  `json:"id"`
	AccountID string                  `json:"account_id"`
	Token     string                  `json:"token"`
	Scopes    []coreingest.TokenScope `json:"scopes"`
}

func tokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func decodeScopes(raw json.RawMessage) ([]coreingest.TokenScope, error) {
	var scopes []coreingest.TokenScope
	if err := json.Unmarshal(raw, &scopes); err != nil {
		return nil, fmt.Errorf("decode token scopes: %w", err)
	}
	return normalizeScopes(scopes)
}

func normalizeScopes(scopes []coreingest.TokenScope) ([]coreingest.TokenScope, error) {
	if len(scopes) == 0 {
		scopes = []coreingest.TokenScope{coreingest.ScopeStateWrite}
	}
	seen := map[coreingest.TokenScope]bool{}
	for _, scope := range scopes {
		switch scope {
		case coreingest.ScopeStateWrite, coreingest.ScopeItemsBank, coreingest.ScopeItemsInventory, coreingest.ScopeItemsEquipment,
			coreingest.ScopeMarksWrite:
		default:
			return nil, fmt.Errorf("unknown token scope %q", scope)
		}
		seen[scope] = true
	}
	seen[coreingest.ScopeStateWrite] = true
	out := make([]coreingest.TokenScope, 0, len(seen))
	for scope := range seen {
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func encodeScopes(scopes []coreingest.TokenScope) ([]byte, error) {
	raw, err := json.Marshal(scopes)
	if err != nil {
		return nil, fmt.Errorf("encode token scopes: %w", err)
	}
	return raw, nil
}

func randomText(prefix string, bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// CreateToken creates a token for an existing active account. The plaintext is
// returned only from this call; only its SHA-256 hash is stored.
func (r *Repository) CreateToken(ctx context.Context, accountID, label string, scopes []coreingest.TokenScope) (CreatedToken, error) {
	account, err := r.queries.GetAccount(ctx, accountID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return CreatedToken{}, ErrAccountNotFound
		}
		return CreatedToken{}, err
	}
	if !account.Active {
		return CreatedToken{}, ErrAccountNotFound
	}
	scopes, err = normalizeScopes(scopes)
	if err != nil {
		return CreatedToken{}, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	plaintext, err := randomText("ipl_", 32)
	if err != nil {
		return CreatedToken{}, fmt.Errorf("generate token: %w", err)
	}
	id, err := randomText("pit_", 16)
	if err != nil {
		return CreatedToken{}, fmt.Errorf("generate token id: %w", err)
	}
	rawScopes, err := encodeScopes(scopes)
	if err != nil {
		return CreatedToken{}, err
	}
	_, err = r.queries.CreatePluginToken(ctx, storedb.CreatePluginTokenParams{
		ID:        id,
		AccountID: accountID,
		TokenHash: text(tokenHash(plaintext)),
		Label: pgtype.Text{
			String: strings.TrimSpace(label),
			Valid:  strings.TrimSpace(label) != "",
		},
		Scopes: rawScopes,
	})
	if err != nil {
		return CreatedToken{}, fmt.Errorf("create plugin token: %w", err)
	}
	return CreatedToken{ID: id, AccountID: accountID, Token: plaintext, Scopes: scopes}, nil
}

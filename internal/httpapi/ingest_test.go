package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/core/content"
	coreingest "github.com/FelipeMalacarne/chronicle/internal/core/ingest"
	storeingest "github.com/FelipeMalacarne/chronicle/internal/store/ingest"
)

type fakeIngestService struct {
	mu           sync.Mutex
	lastSeq      map[string]int64
	accepted     int
	observations []coreingest.ItemObservation
	created      int
	// creds records every credential the handlers passed on.
	creds []storeingest.Credential
}

// testInstallation is a plugin installation id (tokenless dev requests).
const testInstallation = "3f2b8c1e-9a4d-4e7b-8c2a-1d5e6f7a8b9c"

func newFakeIngestService() *fakeIngestService {
	return &fakeIngestService{lastSeq: map[string]int64{}}
}

func testManifest(bank bool) coreingest.Manifest {
	scopes := []coreingest.TokenScope{coreingest.ScopeStateWrite}
	if bank {
		scopes = append(scopes, coreingest.ScopeItemsBank)
	}
	return coreingest.Manifest{
		ContractVersion: coreingest.ContractVersion,
		ManifestVersion: 7,
		TokenScopes:     scopes,
		Varbits:         []int64{},
		Varps:           []int64{},
		Collections:     []int64{},
		Skills:          true,
		Items:           coreingest.ItemSelection{Bank: bank},
		IntervalSeconds: 10,
	}
}

func (f *fakeIngestService) record(c storeingest.Credential) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creds = append(f.creds, c)
}

func (f *fakeIngestService) ManifestFor(_ context.Context, c storeingest.Credential) (coreingest.Manifest, error) {
	f.record(c)
	return f.manifest(c)
}

// Principal: a dev installation acts for c.Account or acc_dev with every
// scope; "valid" and "nobank" read only; "marks" may write marks.
func (f *fakeIngestService) Principal(_ context.Context, c storeingest.Credential) (storeingest.Principal, error) {
	f.record(c)
	if c.Tokenless() {
		account := c.Account
		if account == "ambiguous" {
			return storeingest.Principal{}, storeingest.ErrAccountAmbiguous
		}
		if account == "" {
			account = "acc_dev"
		}
		return storeingest.Principal{AccountID: account, Scopes: []coreingest.TokenScope{coreingest.ScopeStateWrite, coreingest.ScopeMarksWrite}, Tokenless: true}, nil
	}
	switch c.Token {
	case "valid", "nobank":
		return storeingest.Principal{AccountID: "account-1", Scopes: []coreingest.TokenScope{coreingest.ScopeStateWrite}}, nil
	case "marks":
		return storeingest.Principal{AccountID: "account-1", Scopes: []coreingest.TokenScope{coreingest.ScopeStateWrite, coreingest.ScopeMarksWrite}}, nil
	default:
		return storeingest.Principal{}, storeingest.ErrUnauthorized
	}
}

func (f *fakeIngestService) manifest(c storeingest.Credential) (coreingest.Manifest, error) {
	if c.Tokenless() {
		return testManifest(true), nil
	}
	switch c.Token {
	case "valid":
		return testManifest(true), nil
	case "nobank":
		return testManifest(false), nil
	default:
		return coreingest.Manifest{}, storeingest.ErrUnauthorized
	}
}

func (f *fakeIngestService) AcceptFor(_ context.Context, c storeingest.Credential, _ string, _ json.RawMessage, push coreingest.Push) (coreingest.Response, error) {
	f.record(c)
	f.mu.Lock()
	defer f.mu.Unlock()
	manifest, err := f.manifest(c)
	if err != nil {
		return coreingest.Response{}, err
	}
	token := c.Token
	if c.Tokenless() {
		token = "dev:" + c.Installation
	}
	response := coreingest.Response{
		ContractVersion: coreingest.ContractVersion,
		AcceptedSeq:     f.lastSeq[token],
		ManifestVersion: manifest.ManifestVersion,
	}
	if push.ClientSeq <= f.lastSeq[token] {
		response.Resync = true
		return response, storeingest.ErrStaleSequence
	}
	if push.ManifestVersion != manifest.ManifestVersion {
		response.Resync = true
		return response, storeingest.ErrStaleManifest
	}
	if push.Data.Items != nil && push.Data.Items.Bank != nil && push.Data.Items.Bank.State != coreingest.ItemUnavailable && !manifest.Items.Bank {
		return response, storeingest.ErrScopeDenied
	}
	f.lastSeq[token] = push.ClientSeq
	f.accepted++
	if push.Data.Items != nil && push.Data.Items.Bank != nil {
		f.observations = append(f.observations, *push.Data.Items.Bank)
	}
	response.OK = true
	response.AcceptedSeq = push.ClientSeq
	return response, nil
}

func (f *fakeIngestService) CreateToken(_ context.Context, accountID, _ string, scopes []coreingest.TokenScope) (storeingest.CreatedToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if accountID == "missing" {
		return storeingest.CreatedToken{}, storeingest.ErrAccountNotFound
	}
	f.created++
	if len(scopes) == 0 {
		scopes = []coreingest.TokenScope{coreingest.ScopeStateWrite}
	}
	return storeingest.CreatedToken{ID: "pit_test", AccountID: accountID, Token: "ipl_plaintext_once", Scopes: scopes}, nil
}

func newIngestTestServer(t *testing.T, service *fakeIngestService, devMode bool) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(New(Options{
		Version: "test", Content: &content.Content{}, Ingest: service, DevMode: devMode, Logger: logger,
	}))
	t.Cleanup(server.Close)
	return server
}

func intMap(values map[string]int64) *coreingest.IntMap {
	value := coreingest.IntMap(values)
	return &value
}

func quantityMap(values map[string]int64) *coreingest.QuantityMap {
	value := coreingest.QuantityMap(values)
	return &value
}

func unavailable(at time.Time) *coreingest.ItemObservation {
	return &coreingest.ItemObservation{State: coreingest.ItemUnavailable, ObservedAt: at}
}

func fullPush(seq int64, bank coreingest.ItemObservation) coreingest.Push {
	sentAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
	bank.ObservedAt = sentAt.Add(-time.Second)
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion,
		Username:        "Example",
		Profile:         "STANDARD",
		ManifestVersion: 7,
		ClientSeq:       seq,
		Kind:            coreingest.PushFull,
		SentAt:          sentAt,
		Data: coreingest.PushData{
			Varbits: intMap(map[string]int64{}), Varps: intMap(map[string]int64{}),
			Levels: intMap(map[string]int64{}), XP: intMap(map[string]int64{}), Boosted: intMap(map[string]int64{}),
			Items: &coreingest.ItemContainers{Bank: &bank, Inventory: unavailable(sentAt.Add(-time.Second)), Equipment: unavailable(sentAt.Add(-time.Second))},
		},
	}
}

func liveBank(revision int64, epoch string, quantities map[string]int64) coreingest.ItemObservation {
	return coreingest.ItemObservation{State: coreingest.ItemLive, Revision: &revision, LiveEpoch: epoch, Quantities: quantityMap(quantities)}
}

func restoredBank(revision int64, quantities map[string]int64) coreingest.ItemObservation {
	return coreingest.ItemObservation{State: coreingest.ItemRestored, Revision: &revision, Quantities: quantityMap(quantities)}
}

func deltaPush(seq int64, bank coreingest.ItemObservation) coreingest.Push {
	sentAt := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Minute)
	bank.ObservedAt = sentAt.Add(-time.Second)
	return coreingest.Push{
		ContractVersion: coreingest.ContractVersion,
		Username:        "Example",
		Profile:         "STANDARD",
		ManifestVersion: 7,
		ClientSeq:       seq,
		Kind:            coreingest.PushDelta,
		SentAt:          sentAt,
		Data:            coreingest.PushData{Items: &coreingest.ItemContainers{Bank: &bank}},
	}
}

func ingestRequest(t *testing.T, server *httptest.Server, method, path, token string, body any) (*http.Response, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(raw))
	}
	request, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return response, raw
}

func TestIngestAuthentication(t *testing.T) {
	server := newIngestTestServer(t, newFakeIngestService(), false)
	for _, tc := range []struct {
		name, token string
	}{
		{name: "missing"},
		{name: "revoked", token: "revoked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, _ := ingestRequest(t, server, http.MethodGet, "/api/ingest/manifest", tc.token, nil)
			if response.StatusCode != http.StatusUnauthorized || response.Header.Get("WWW-Authenticate") == "" {
				t.Fatalf("status = %d, authenticate = %q", response.StatusCode, response.Header.Get("WWW-Authenticate"))
			}
		})
	}
	response, raw := ingestRequest(t, server, http.MethodGet, "/api/ingest/manifest", "valid", nil)
	var manifest coreingest.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || manifest.ManifestVersion != 7 || !manifest.Items.Bank {
		t.Fatalf("valid manifest = %d %+v", response.StatusCode, manifest)
	}
}

func TestIngestStaleSequenceAndManifest(t *testing.T) {
	service := newFakeIngestService()
	server := newIngestTestServer(t, service, false)
	push := fullPush(1, restoredBank(4, map[string]int64{"4151": 1}))
	if response, raw := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", push); response.StatusCode != http.StatusOK {
		t.Fatalf("initial push = %d: %s", response.StatusCode, raw)
	}
	response, raw := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", push)
	var stale coreingest.Response
	if err := json.Unmarshal(raw, &stale); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict || stale.OK || stale.AcceptedSeq != 1 || !stale.Resync {
		t.Fatalf("stale sequence = %d %+v", response.StatusCode, stale)
	}
	push.ClientSeq = 2
	push.ManifestVersion = 6
	response, raw = ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", push)
	if err := json.Unmarshal(raw, &stale); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusConflict || stale.AcceptedSeq != 1 || !stale.Resync {
		t.Fatalf("stale manifest = %d %+v", response.StatusCode, stale)
	}
	if service.accepted != 1 {
		t.Fatalf("accepted writes = %d, want 1", service.accepted)
	}
}

func TestIngestDisabledBankAndFreshness(t *testing.T) {
	service := newFakeIngestService()
	server := newIngestTestServer(t, service, false)
	push := fullPush(1, restoredBank(3, map[string]int64{"995": 20}))
	if response, _ := ingestRequest(t, server, http.MethodPost, "/api/ingest", "nobank", push); response.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled bank status = %d", response.StatusCode)
	}
	if response, raw := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", push); response.StatusCode != http.StatusOK {
		t.Fatalf("restored status = %d: %s", response.StatusCode, raw)
	}
	live := deltaPush(2, liveBank(0, "login-1", map[string]int64{"995": 21}))
	if response, raw := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", live); response.StatusCode != http.StatusOK {
		t.Fatalf("live status = %d: %s", response.StatusCode, raw)
	}
	if len(service.observations) != 2 || service.observations[0].State != coreingest.ItemRestored || service.observations[0].LiveEpoch != "" || service.observations[1].State != coreingest.ItemLive || service.observations[1].LiveEpoch != "login-1" {
		t.Fatalf("observations = %+v", service.observations)
	}
}

func TestIngestFullDeltaZeroAndIdempotency(t *testing.T) {
	service := newFakeIngestService()
	server := newIngestTestServer(t, service, false)
	invalid := fullPush(1, liveBank(1, "login-1", map[string]int64{"4151": 0}))
	if response, _ := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", invalid); response.StatusCode != http.StatusBadRequest {
		t.Fatalf("full zero status = %d", response.StatusCode)
	}
	initial := fullPush(1, liveBank(1, "login-1", map[string]int64{"4151": 1}))
	if response, raw := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", initial); response.StatusCode != http.StatusOK {
		t.Fatalf("full status = %d: %s", response.StatusCode, raw)
	}
	tombstone := deltaPush(2, liveBank(2, "login-1", map[string]int64{"4151": 0}))
	if response, raw := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", tombstone); response.StatusCode != http.StatusOK {
		t.Fatalf("delta zero status = %d: %s", response.StatusCode, raw)
	}
	if got := (*service.observations[1].Quantities)["4151"]; got != 0 {
		t.Fatalf("delta quantity = %d", got)
	}
	if response, _ := ingestRequest(t, server, http.MethodPost, "/api/ingest", "valid", tombstone); response.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate status = %d", response.StatusCode)
	}
	if service.accepted != 2 {
		t.Fatalf("accepted writes = %d, want 2", service.accepted)
	}
}

func TestDevIngestTokenBootstrap(t *testing.T) {
	service := newFakeIngestService()
	production := newIngestTestServer(t, service, false)
	if response, _ := ingestRequest(t, production, http.MethodPost, "/api/dev/ingest-token", "", map[string]any{"account_id": "account-1"}); response.StatusCode != http.StatusNotFound {
		t.Fatalf("production status = %d", response.StatusCode)
	}

	development := newIngestTestServer(t, service, true)
	response, raw := ingestRequest(t, development, http.MethodPost, "/api/dev/ingest-token", "", map[string]any{"account_id": "account-1"})
	if response.StatusCode != http.StatusCreated || response.Header.Get("Cache-Control") != "no-store" || !strings.Contains(string(raw), "ipl_plaintext_once") {
		t.Fatalf("development token = %d cache=%q body=%s", response.StatusCode, response.Header.Get("Cache-Control"), raw)
	}
}

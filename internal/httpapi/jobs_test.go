package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2/memstore"

	"github.com/FelipeMalacarne/chronicle/internal/auth"
	"github.com/FelipeMalacarne/chronicle/internal/auth/authtest"
	"github.com/FelipeMalacarne/chronicle/internal/core/job"
	"github.com/FelipeMalacarne/chronicle/internal/store/eventlog"
	storejobs "github.com/FelipeMalacarne/chronicle/internal/store/jobs"
)

// memJobs is JobStore in memory.
type memJobs struct {
	jobs []job.Job
}

func (m *memJobs) Create(_ context.Context, j job.Job) (job.Job, error) {
	j.ID = fmt.Sprintf("job_%016x", len(m.jobs)+1)
	j.CreatedAt = time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	m.jobs = append(m.jobs, j)
	return j, nil
}

func (m *memJobs) find(account, id string) (*job.Job, error) {
	for i := range m.jobs {
		if m.jobs[i].ID == id && m.jobs[i].Account == account {
			return &m.jobs[i], nil
		}
	}
	return nil, storejobs.ErrNotFound
}

func (m *memJobs) Get(_ context.Context, account, id string) (job.Job, error) {
	j, err := m.find(account, id)
	if err != nil {
		return job.Job{}, err
	}
	return *j, nil
}

func (m *memJobs) List(_ context.Context, account, state string, _ int) ([]job.Job, error) {
	out := []job.Job{}
	for _, j := range m.jobs {
		if j.Account == account && (state == "" || string(j.State) == state) {
			out = append(out, j)
		}
	}
	return out, nil
}

func (m *memJobs) Cancel(_ context.Context, account, id, reason string) (job.Job, error) {
	j, err := m.find(account, id)
	if err != nil {
		return job.Job{}, err
	}
	if err := job.Check(j.State, job.Canceled); err != nil {
		return job.Job{}, storejobs.ErrState
	}
	j.State, j.Reason = job.Canceled, reason
	return *j, nil
}

func (m *memJobs) ActiveRuns(context.Context, string) ([]string, error) { return nil, nil }

func (m *memJobs) ProvenTasks(context.Context, string, string) (map[string]string, error) {
	return nil, nil
}

type memJobEvents struct{}

func (memJobEvents) OfJob(_ context.Context, account, id string, _ int) ([]eventlog.Event, error) {
	return []eventlog.Event{{ID: 1, Account: account, Type: eventlog.TypeJobQueued, Subject: id, Payload: json.RawMessage(`{}`)}}, nil
}

const newJob = `{"kind":"plugin","goal":{"type":"varp","id":281,"min":1000},"spec":{"type":"plugin","plugin":{"plugin":"net.runelite.client.plugins.microbot.tutorialisland.TutorialIslandPlugin"}}}`

func jobsServer(t *testing.T) (*httptest.Server, *memJobs) {
	t.Helper()
	store := &memJobs{}
	srv := httptest.NewServer(New(Options{
		Version: "test", DevMode: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Jobs: store, JobEvents: memJobEvents{},
	}))
	t.Cleanup(srv.Close)
	return srv, store
}

func TestJobRoutes(t *testing.T) {
	srv, _ := jobsServer(t)
	url := srv.URL + "/api/jobs"
	status, body, _ := call(t, "POST", url+"?account=acc-1", newJob)
	if status != http.StatusCreated || body["state"] != "queued" || body["source"] != "owner" || body["kind"] != "plugin" {
		t.Fatalf("create = %d %v", status, body)
	}
	if m := body["metric"].(map[string]any); m["kind"] != "varp" || m["target"].(float64) != 1000 {
		t.Fatalf("the metric defaults to the goal's: %v", m)
	}
	if body["stall_after_s"].(float64) != float64(30*60) {
		t.Fatalf("stall_after_s = %v", body["stall_after_s"])
	}
	id := body["id"].(string)

	status, body, _ = call(t, "GET", url+"?account=acc-1", "")
	if jobs, _ := body["jobs"].([]any); status != http.StatusOK || len(jobs) != 1 {
		t.Fatalf("list = %d %v", status, body)
	}
	if _, body, _ = call(t, "GET", url+"?account=acc-1&state=running", ""); len(body["jobs"].([]any)) != 0 {
		t.Fatalf("running = %v", body)
	}
	if _, body, _ = call(t, "GET", url+"?account=acc-2", ""); len(body["jobs"].([]any)) != 0 {
		t.Fatalf("another account's jobs = %v", body)
	}
	if status, _, _ = call(t, "GET", url+"?account=acc-1&state=nope", ""); status != http.StatusBadRequest {
		t.Fatalf("bad state = %d", status)
	}
	if status, _, _ = call(t, "GET", url+"?account=acc-1&limit=0", ""); status != http.StatusBadRequest {
		t.Fatalf("bad limit = %d", status)
	}

	status, body, _ = call(t, "GET", url+"/"+id+"?account=acc-1", "")
	if status != http.StatusOK || body["job"].(map[string]any)["id"] != id || len(body["events"].([]any)) != 1 || len(body["runs"].([]any)) != 0 {
		t.Fatalf("get = %d %v", status, body)
	}
	if status, _, _ = call(t, "GET", url+"/"+id+"?account=acc-2", ""); status != http.StatusNotFound {
		t.Fatalf("another account's job = %d", status)
	}

	if status, _, _ = call(t, "POST", url+"/"+id+"/cancel?account=acc-2", ""); status != http.StatusNotFound {
		t.Fatalf("cancel on another account = %d", status)
	}
	status, body, _ = call(t, "POST", url+"/"+id+"/cancel?account=acc-1", "")
	if status != http.StatusOK || body["job"].(map[string]any)["state"] != "canceled" || body["run_canceled"] != false {
		t.Fatalf("cancel = %d %v", status, body)
	}
	if status, _, _ = call(t, "POST", url+"/"+id+"/cancel?account=acc-1", ""); status != http.StatusConflict {
		t.Fatalf("cancel again = %d", status)
	}
}

func TestCreateJobRefusals(t *testing.T) {
	srv, store := jobsServer(t)
	for name, body := range map[string]string{
		"no goal":       `{"kind":"plugin"}`,
		"bad kind":      `{"kind":"fly","goal":"skill:mining>=15"}`,
		"bad goal":      `{"kind":"train","goal":{"type":"nope"}}`,
		"loose key":     `{"kind":"train","goal":"skill:mining>=015"}`,
		"short stall":   `{"kind":"train","goal":"skill:mining>=15","stall_after_s":5}`,
		"bad metric":    `{"kind":"train","goal":"skill:mining>=15","metric":{"kind":"magic","target":1}}`,
		"not json":      `{`,
		"unknown field": `{"kind":"train","goal":"skill:mining>=15","state":"succeeded"}`,
		"a plan task":   `{"kind":"ge_sell","goal":"skill:mining>=15","plan":"pln_abcdefgh","task_key":"task:0#0"}`,
	} {
		if status, _, _ := call(t, "POST", srv.URL+"/api/jobs?account=acc-1", body); status != http.StatusBadRequest {
			t.Errorf("%s = %d", name, status)
		}
	}
	if len(store.jobs) != 0 {
		t.Fatalf("a refused job was stored: %+v", store.jobs)
	}
	// An objective key as the goal gets its metric.
	status, body, _ := call(t, "POST", srv.URL+"/api/jobs?account=acc-1", `{"kind":"train","goal":"skill:mining>=15"}`)
	if status != http.StatusCreated || body["metric"].(map[string]any)["kind"] != "skill_xp" {
		t.Fatalf("train job = %d %v", status, body)
	}
}

// The job routes are account routes (rule 10): in oidc they need a session,
// reach only the session user's accounts, and the writes need the CSRF header
// (cancel, which acts in game, also the Microbot role);
// outside dev mode and oidc they do not exist.
func TestJobRoutesOIDC(t *testing.T) {
	store := &memJobs{}
	e := newOIDCEnv(t, newMemIdentity(), memstore.New(), auth.Policy{RequiredRole: "owner"}, func(o *Options) { o.Jobs = store })
	c := e.client()
	if status, _ := e.do(c, "GET", "/api/jobs?account=acc-1", "", false); status != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d", status)
	}
	e.login(c, "/", owner)
	if status, _ := e.do(c, "GET", "/api/jobs?account=acc-2", "", false); status != http.StatusNotFound {
		t.Fatalf("another user's account = %d", status)
	}
	if status, _ := e.do(c, "POST", "/api/jobs?account=acc-1", newJob, false); status != http.StatusForbidden {
		t.Fatalf("create without CSRF = %d", status)
	}
	if status, _ := e.do(c, "POST", "/api/jobs?account=acc-2", newJob, true); status != http.StatusNotFound || len(store.jobs) != 0 {
		t.Fatalf("create on another user's account = %d", status)
	}
	status, body := e.do(c, "POST", "/api/jobs?account=acc-1", newJob, true)
	if status != http.StatusCreated || len(store.jobs) != 1 {
		t.Fatalf("create = %d %v", status, body)
	}
	id := body["id"].(string)
	if status, _ := e.do(c, "POST", "/api/jobs/"+id+"/cancel?account=acc-1", "", false); status != http.StatusForbidden {
		t.Fatalf("cancel without CSRF = %d", status)
	}
	if status, _ := e.do(c, "POST", "/api/jobs/"+id+"/cancel?account=acc-1", "", true); status != http.StatusForbidden {
		t.Fatalf("cancel without the %s role = %d", MicrobotRole, status)
	}
	e.login(c, "/", authtest.User{Subject: owner.Subject, Username: owner.Username, Roles: []string{"owner", MicrobotRole}})
	if status, _ := e.do(c, "POST", "/api/jobs/"+id+"/cancel?account=acc-1", "", true); status != http.StatusOK {
		t.Fatalf("cancel = %d", status)
	}
}

func TestJobRoutesAreDevOnly(t *testing.T) {
	srv := httptest.NewServer(New(Options{Version: "test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Jobs: &memJobs{}}))
	t.Cleanup(srv.Close)
	if status, _, _ := call(t, "GET", srv.URL+"/api/jobs?account=acc-1", ""); status != http.StatusNotFound {
		t.Fatalf("outside dev mode and oidc = %d", status)
	}
}

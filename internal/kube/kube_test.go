package kube

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkerName(t *testing.T) {
	for in, want := range map[string]string{
		"acc_iron_man":          "worker-acc-iron-man",
		"ACC_X":                 "worker-acc-x",
		"acc_a.b":               "worker-acc-a-b",
		"_acc_":                 "worker-acc",
		strings.Repeat("a", 60): "worker-" + strings.Repeat("a", 40),
	} {
		if got := WorkerName(in); got != want {
			t.Errorf("WorkerName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkerDeployment(t *testing.T) {
	d := WorkerDeployment("chronicle", WorkerSpec{Account: "acc_iron_man", Image: "ghcr.io/felipemalacarne/chronicle-worker:abc", ServerURL: "http://chronicle.chronicle.svc", Version: "abc"})
	if d.Metadata.Name != "worker-acc-iron-man" || d.Spec.Strategy.Type != "Recreate" || *d.Spec.Replicas != 1 {
		t.Fatalf("deployment: %+v", d.Metadata)
	}
	pod := d.Spec.Template.Spec
	if pod.ServiceAccountName != WorkerServiceAccount || *pod.AutomountServiceAccountToken {
		t.Fatal("pod must use the worker service account without the automounted token")
	}
	if d.Spec.Template.Metadata.Labels[LabelAccount] != "acc_iron_man" || d.Spec.Selector.MatchLabels[LabelComponent] != ComponentWork {
		t.Fatalf("labels: %v", d.Spec.Template.Metadata.Labels)
	}
	// The restart annotation is in the pod template only when asked for: a
	// start or a new image must not carry an old one.
	if len(d.Spec.Template.Metadata.Annotations) != 0 {
		t.Fatalf("pod annotations without a restart: %v", d.Spec.Template.Metadata.Annotations)
	}
	r := WorkerDeployment("chronicle", WorkerSpec{Account: "acc_iron_man", Image: "img", RestartedAt: "2026-11-13T03:30:00Z"})
	if r.Spec.Template.Metadata.Annotations[AnnotationRestartedAt] != "2026-11-13T03:30:00Z" {
		t.Fatalf("restart annotation: %v", r.Spec.Template.Metadata.Annotations)
	}
	env := map[string]string{}
	for _, e := range pod.Containers[0].Env {
		env[e.Name] = e.Value
	}
	if env["MB_HEADLESS_SIZE"] != WorkerDisplay || pod.Containers[0].Resources.Requests["cpu"] != "700m" {
		t.Fatalf("display %q, cpu request %q", env["MB_HEADLESS_SIZE"], pod.Containers[0].Resources.Requests["cpu"])
	}
	if env["WORKER_GAME_CACHE"] != workerCacheDir || strings.HasPrefix(workerCacheDir, workerHome+"/") {
		t.Fatalf("the game cache must be mounted outside the home: %q", env["WORKER_GAME_CACHE"])
	}
	for _, m := range pod.Containers[0].VolumeMounts {
		if m.Name == "game-cache" && m.MountPath != workerCacheDir {
			t.Fatalf("game-cache mounted at %s", m.MountPath)
		}
	}
	if env["WORKER_ACCOUNT"] != "acc_iron_man" || env["CHRONICLE_URL"] != "http://chronicle.chronicle.svc" || env["MB_HWID_STORE_DIR"] != workerSecrets {
		t.Fatalf("env: %v", env)
	}
	var claim, token bool
	for _, v := range pod.Volumes {
		if v.PersistentVolumeClaim != nil && v.PersistentVolumeClaim.ClaimName == CacheClaim {
			claim = true
		}
		if v.Projected != nil && v.Projected.Sources[0].ServiceAccountToken.Audience == WorkerAudience {
			token = true
		}
		if v.Name == "secrets" && v.EmptyDir.Medium != "Memory" {
			t.Fatal("the HWID stores must live in memory")
		}
	}
	if !claim || !token {
		t.Fatalf("volumes: %+v", pod.Volumes)
	}
	// Server-side apply must not send a status.
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), `"status"`) {
		t.Fatalf("apply body carries a status: %s", b)
	}
}

func TestPodState(t *testing.T) {
	waiting := func(reason string) Pod {
		var p Pod
		cs := ContainerStatus{}
		cs.State.Waiting = &struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		}{Reason: reason}
		p.Status.ContainerStatuses = []ContainerStatus{cs}
		return p
	}
	unscheduled := Pod{Status: PodStatus{Phase: "Pending", Conditions: []PodCondition{{Type: "PodScheduled", Status: "False", Reason: "Unschedulable"}}}}
	ready := Pod{Status: PodStatus{Phase: "Running", ContainerStatuses: []ContainerStatus{{Ready: true}}}}
	deleting := Pod{Metadata: ObjectMeta{DeletionTimestamp: "2026-11-12T00:00:00Z"}}
	tests := []struct {
		pod           Pod
		state, reason string
	}{
		{unscheduled, PodPending, "Unschedulable"},
		{waiting("ContainerCreating"), PodPulling, "ContainerCreating"},
		{waiting("ImagePullBackOff"), PodFailing, "ImagePullBackOff"},
		{waiting("CrashLoopBackOff"), PodFailing, "CrashLoopBackOff"},
		{ready, PodRunning, ""},
		{deleting, PodTerminating, ""},
	}
	for i, tt := range tests {
		if s, r := PodState(tt.pod); s != tt.state || r != tt.reason {
			t.Errorf("%d: got %s/%s, want %s/%s", i, s, r, tt.state, tt.reason)
		}
	}
}

// fakeAPI is an API server that records requests and answers from handlers.
func fakeAPI(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sa-token" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{Base: srv.URL, Namespace: "chronicle", Token: func() (string, error) { return "sa-token", nil }, HTTP: srv.Client()}
}

func TestApplyDeleteList(t *testing.T) {
	var applied Deployment
	c := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/apis/apps/v1/namespaces/chronicle/deployments/worker-acc-x":
			if r.Header.Get("Content-Type") != contentApply || r.URL.Query().Get("fieldManager") != FieldManager || r.URL.Query().Get("force") != "true" {
				http.Error(w, "bad apply", http.StatusBadRequest)
				return
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &applied)
			_, _ = w.Write(b)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/worker-gone"):
			http.Error(w, `{"reason":"NotFound","message":"not found"}`, http.StatusNotFound)
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{}`))
		case r.Method == http.MethodGet && r.URL.Path == "/apis/apps/v1/namespaces/chronicle/deployments":
			if r.URL.Query().Get("labelSelector") != WorkerSelector {
				http.Error(w, "bad selector", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"items":[{"metadata":{"name":"worker-acc-x","labels":{"chronicle.ftm.dev.br/account":"acc_x"}},"spec":{"strategy":{"type":"Recreate"},"selector":{"matchLabels":{}},"template":{"metadata":{},"spec":{"containers":[{"name":"worker","image":"img"}]}}},"status":{"readyReplicas":1}}]}`))
		case r.Method == http.MethodPatch:
			http.Error(w, `{"reason":"Invalid","message":"denied by policy"}`, http.StatusUnprocessableEntity)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusTeapot)
		}
	})
	ctx := context.Background()
	if err := c.ApplyDeployment(ctx, WorkerDeployment("chronicle", WorkerSpec{Account: "acc_x", Image: "img"})); err != nil {
		t.Fatal(err)
	}
	if applied.Metadata.Labels[LabelAccount] != "acc_x" {
		t.Fatalf("applied: %+v", applied.Metadata)
	}
	var api *APIError
	if err := c.ApplyDeployment(ctx, WorkerDeployment("chronicle", WorkerSpec{Account: "acc_y", Image: "img"})); !errors.As(err, &api) || api.Status != 422 || api.Message != "denied by policy" {
		t.Fatalf("denied apply: %v", err)
	}
	if err := c.DeleteDeployment(ctx, "worker-acc-x"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteDeployment(ctx, "worker-gone"); err != nil {
		t.Fatalf("a missing deployment is not an error: %v", err)
	}
	ds, err := c.ListDeployments(ctx, WorkerSelector)
	if err != nil || len(ds) != 1 || ds[0].Status.ReadyReplicas != 1 || ds[0].Metadata.Labels[LabelAccount] != "acc_x" {
		t.Fatalf("list: %+v %v", ds, err)
	}
}

func TestVerifyWorkerToken(t *testing.T) {
	pods := map[string]string{
		"worker-acc-x-1": `{"metadata":{"name":"worker-acc-x-1","uid":"u1","labels":{"app.kubernetes.io/name":"chronicle","app.kubernetes.io/component":"worker","chronicle.ftm.dev.br/account":"acc_x"}},"spec":{"serviceAccountName":"chronicle-worker","containers":[]},"status":{}}`,
		"web-1":          `{"metadata":{"name":"web-1","uid":"u2","labels":{"app.kubernetes.io/name":"chronicle","app.kubernetes.io/component":"web"}},"spec":{"serviceAccountName":"chronicle-worker","containers":[]},"status":{}}`,
	}
	reviews := map[string]string{
		"good":     `{"status":{"authenticated":true,"user":{"username":"system:serviceaccount:chronicle:chronicle-worker","extra":{"authentication.kubernetes.io/pod-name":["worker-acc-x-1"],"authentication.kubernetes.io/pod-uid":["u1"]}}}}`,
		"replaced": `{"status":{"authenticated":true,"user":{"username":"system:serviceaccount:chronicle:chronicle-worker","extra":{"authentication.kubernetes.io/pod-name":["worker-acc-x-1"],"authentication.kubernetes.io/pod-uid":["old"]}}}}`,
		"server":   `{"status":{"authenticated":true,"user":{"username":"system:serviceaccount:chronicle:chronicle","extra":{"authentication.kubernetes.io/pod-name":["worker-acc-x-1"]}}}}`,
		"web":      `{"status":{"authenticated":true,"user":{"username":"system:serviceaccount:chronicle:chronicle-worker","extra":{"authentication.kubernetes.io/pod-name":["web-1"]}}}}`,
		"unbound":  `{"status":{"authenticated":true,"user":{"username":"system:serviceaccount:chronicle:chronicle-worker"}}}`,
		"bad":      `{"status":{"authenticated":false,"error":"token expired"}}`,
	}
	c := fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/apis/authentication.k8s.io/v1/tokenreviews":
			var body struct {
				Spec struct {
					Token     string   `json:"token"`
					Audiences []string `json:"audiences"`
				} `json:"spec"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Spec.Audiences) != 1 || body.Spec.Audiences[0] != WorkerAudience {
				http.Error(w, "audience", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(reviews[body.Spec.Token]))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/namespaces/chronicle/pods/"):
			p, ok := pods[strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/chronicle/pods/")]
			if !ok {
				http.Error(w, "{}", http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(p))
		default:
			http.Error(w, "unexpected", http.StatusTeapot)
		}
	})
	id, err := c.VerifyWorkerToken(context.Background(), "good")
	if err != nil || id.Pod != "worker-acc-x-1" || id.Account != "acc_x" {
		t.Fatalf("good: %+v %v", id, err)
	}
	for _, tok := range []string{"replaced", "server", "web", "unbound", "bad"} {
		if _, err := c.VerifyWorkerToken(context.Background(), tok); err == nil {
			t.Errorf("%s: accepted", tok)
		}
	}
}

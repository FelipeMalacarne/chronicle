package workerctl

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/kube"
)

func quota(hard, used map[string]string) kube.ResourceQuota {
	return kube.ResourceQuota{Spec: kube.ResourceQuotaSpec{Hard: hard}, Status: kube.ResourceQuotaStatus{Hard: hard, Used: used}}
}

func TestCapacityOf(t *testing.T) {
	now := time.Date(2026, 11, 13, 4, 0, 0, 0, time.UTC)
	// The saradomin quota on 2026-11-13: the app (20m, 64Mi) and one worker.
	saradomin := quota(
		map[string]string{"count/deployments.apps": "4", "requests.cpu": "2200m", "requests.memory": "3328Mi"},
		map[string]string{"count/deployments.apps": "2", "requests.cpu": "720m", "requests.memory": "1088Mi"},
	)
	tests := []struct {
		name      string
		quotas    []kube.ResourceQuota
		known     bool
		fits      int
		limitedBy string
	}{
		{name: "no quota", quotas: nil},
		{name: "a quota on nothing a worker takes", quotas: []kube.ResourceQuota{quota(map[string]string{"count/secrets": "10"}, map[string]string{"count/secrets": "1"})}},
		{name: "status not computed yet", quotas: []kube.ResourceQuota{{Spec: kube.ResourceQuotaSpec{Hard: map[string]string{"requests.cpu": "2"}}}}},
		{name: "used missing for a resource", quotas: []kube.ResourceQuota{quota(map[string]string{"requests.cpu": "2"}, map[string]string{})}},
		// 1480m / 700m, 2240Mi / 1Gi and 4 - 2 deployments all give 2: the
		// tie goes to CPU.
		{name: "saradomin", quotas: []kube.ResourceQuota{saradomin}, known: true, fits: 2, limitedBy: "requests.cpu"},
		{name: "cpu is the limit", quotas: []kube.ResourceQuota{quota(
			map[string]string{"requests.cpu": "2", "requests.memory": "8Gi"},
			map[string]string{"requests.cpu": "1500m", "requests.memory": "1Gi"},
		)}, known: true, fits: 0, limitedBy: "requests.cpu"},
		{name: "over quota counts as full", quotas: []kube.ResourceQuota{quota(
			map[string]string{"cpu": "1", "memory": "1Gi"},
			map[string]string{"cpu": "1400m", "memory": "512Mi"},
		)}, known: true, fits: 0, limitedBy: "cpu"},
		{name: "limits and pods", quotas: []kube.ResourceQuota{quota(
			map[string]string{"limits.memory": "8Gi", "pods": "10"},
			map[string]string{"limits.memory": "1792Mi", "pods": "1"},
		)}, known: true, fits: 3, limitedBy: "limits.memory"},
		{name: "the tightest of two quotas", quotas: []kube.ResourceQuota{saradomin, quota(
			map[string]string{"count/deployments.apps": "3"},
			map[string]string{"count/deployments.apps": "2"},
		)}, known: true, fits: 1, limitedBy: "count/deployments.apps"},
	}
	for _, tt := range tests {
		c, ok := capacityOf(tt.quotas, now)
		if ok != tt.known {
			t.Errorf("%s: known = %v, want %v (%+v)", tt.name, ok, tt.known, c)
			continue
		}
		if ok && (c.Fits != tt.fits || c.LimitedBy != tt.limitedBy) {
			t.Errorf("%s: fits %d by %s, want %d by %s", tt.name, c.Fits, c.LimitedBy, tt.fits, tt.limitedBy)
		}
	}
	c, _ := capacityOf([]kube.ResourceQuota{saradomin}, now)
	if c.CPUHardCores == nil || *c.CPUHardCores != 2.2 || *c.CPUFreeCores != 1.48 ||
		c.MemoryHardBytes == nil || *c.MemoryHardBytes != 3328<<20 || *c.MemoryFreeBytes != 2240<<20 ||
		c.WorkerCPUCores != 0.7 || c.WorkerMemoryBytes != 1<<30 || !c.Seen.Equal(now) {
		t.Fatalf("saradomin capacity: %+v", c)
	}
	// Without a requests ceiling the CPU and memory parts are absent.
	c, _ = capacityOf([]kube.ResourceQuota{quota(map[string]string{"pods": "4"}, map[string]string{"pods": "1"})}, now)
	if c.Fits != 3 || c.CPUHardCores != nil || c.MemoryFreeBytes != nil {
		t.Fatalf("pods-only capacity: %+v", c)
	}
}

// lockedBuffer is a log sink the test reads while the reconciler writes.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) count(s string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.b.String(), s)
}

func TestReconcileUsageAndCapacity(t *testing.T) {
	store := &fakeStore{desired: []Desired{{Account: "acc_a", Running: true}, {Account: "acc_b", Running: true}}}
	cl := &fakeCluster{
		deps: map[string]kube.Deployment{},
		metrics: map[string]kube.PodMetrics{
			// fakeCluster names each worker's pod <deployment>-abc.
			"worker-acc-a-abc": {
				Metadata: kube.ObjectMeta{Name: "worker-acc-a-abc"}, Timestamp: "2026-11-13T04:03:58Z",
				Containers: []kube.ContainerMetrics{{Name: "worker", Usage: map[string]string{"cpu": "403585713n", "memory": "856788Ki"}}},
			},
			// A pod that is not the account's current one is not its usage.
			"worker-acc-b-old": {
				Metadata:   kube.ObjectMeta{Name: "worker-acc-b-old"},
				Containers: []kube.ContainerMetrics{{Name: "worker", Usage: map[string]string{"cpu": "1", "memory": "1Gi"}}},
			},
		},
		quotas: []kube.ResourceQuota{quota(
			map[string]string{"requests.cpu": "2200m", "requests.memory": "3328Mi"},
			map[string]string{"requests.cpu": "1420m", "requests.memory": "2112Mi"},
		)},
	}
	logs := &lockedBuffer{}
	r := newRec(store, cl, &fakeHub{})
	r.Log = slog.New(slog.NewTextHandler(logs, nil))
	ctx := context.Background()
	if err := r.Once(ctx); err != nil {
		t.Fatal(err)
	}
	usage := r.Usage()
	want := Usage{CPUCores: 0.404, MemoryBytes: 877350912, MeasuredAt: time.Date(2026, 11, 13, 4, 3, 58, 0, time.UTC)}
	if len(usage) != 1 || usage["acc_a"] != want {
		t.Fatalf("usage = %+v", usage)
	}
	if c, ok := r.Capacity(); !ok || c.Fits != 1 || c.LimitedBy != "requests.cpu" {
		t.Fatalf("capacity = %+v %v", c, ok)
	}

	// No metrics-server (or no Role rule): the pass goes on, usage is left
	// out, the capacity stays, and the failure is logged once.
	cl.metricsErr = kube.ErrNotFound
	for range 3 {
		if err := r.Once(ctx); err != nil {
			t.Fatalf("a missing metrics API failed the pass: %v", err)
		}
	}
	if len(r.Usage()) != 0 {
		t.Fatalf("usage without metrics: %+v", r.Usage())
	}
	if _, ok := r.Capacity(); !ok {
		t.Fatal("capacity lost with the metrics")
	}
	if n := logs.count("cannot read pod metrics"); n != 1 {
		t.Fatalf("logged the missing metrics %d times", n)
	}
	cl.metricsErr = nil
	if err := r.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if len(r.Usage()) != 1 || logs.count("pod metrics (metrics.k8s.io) readable again") != 1 {
		t.Fatalf("metrics back: usage %+v", r.Usage())
	}

	// No quota: the capacity is unknown, not zero.
	cl.quotas = nil
	if err := r.Once(ctx); err != nil {
		t.Fatal(err)
	}
	if c, ok := r.Capacity(); ok {
		t.Fatalf("capacity without a quota: %+v", c)
	}
	cl.quotaErr = &kube.APIError{Status: 403, Reason: "Forbidden"}
	if err := r.Once(ctx); err != nil {
		t.Fatalf("a forbidden quota read failed the pass: %v", err)
	}
	if _, ok := r.Capacity(); ok || logs.count("cannot read resource quotas") != 1 {
		t.Fatal("forbidden quotas: capacity kept or not logged")
	}
}

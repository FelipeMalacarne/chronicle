package workerctl

import (
	"math"
	"math/big"
	"time"

	"github.com/FelipeMalacarne/chronicle/internal/kube"
)

// Usage is what the metrics API last measured of a worker's pod.
type Usage struct {
	// CPUCores is the CPU in use, in cores (to the millicore).
	CPUCores float64 `json:"cpu_cores"`
	// MemoryBytes is the working set, the memory the kubelet counts against
	// the limit.
	MemoryBytes int64 `json:"memory_bytes"`
	// MeasuredAt is the end of the metrics window.
	MeasuredAt time.Time `json:"measured_at"`
}

// Capacity is how many more workers the namespace's ResourceQuotas admit
// ("cabe mais N"). Accounts only tracked (no worker) take no room.
type Capacity struct {
	// Fits is the least, over every quota and every resource one worker
	// takes, of floor(free / what one worker takes).
	Fits int `json:"fits"`
	// LimitedBy is the quota resource that gives Fits (requests.cpu,
	// requests.memory, count/deployments.apps, ...).
	LimitedBy string `json:"limited_by"`
	// The requests ceiling of CPU and memory (requests.cpu or cpu,
	// requests.memory or memory) of the tightest quota, and what is free of
	// it; absent when no quota sets one.
	CPUHardCores    *float64 `json:"cpu_hard_cores,omitempty"`
	CPUFreeCores    *float64 `json:"cpu_free_cores,omitempty"`
	MemoryHardBytes *int64   `json:"memory_hard_bytes,omitempty"`
	MemoryFreeBytes *int64   `json:"memory_free_bytes,omitempty"`
	// WorkerCPUCores and WorkerMemoryBytes are what one more worker requests
	// (kube.WorkerResources).
	WorkerCPUCores    float64 `json:"worker_cpu_cores"`
	WorkerMemoryBytes int64   `json:"worker_memory_bytes"`
	// Seen is when the quotas were read.
	Seen time.Time `json:"seen"`
}

// perWorker is what one more worker adds to a quota resource; false for a
// resource the worker does not take (or that the count ignores).
func perWorker(resource string) (*big.Rat, bool) {
	res := kube.WorkerResources()
	var v string
	switch resource {
	case "requests.cpu", "cpu":
		v = res.Requests["cpu"]
	case "requests.memory", "memory":
		v = res.Requests["memory"]
	case "limits.cpu":
		v = res.Limits["cpu"]
	case "limits.memory":
		v = res.Limits["memory"]
	case "pods", "count/pods", "count/deployments.apps", "count/replicasets.apps":
		return big.NewRat(1, 1), true
	default:
		return nil, false
	}
	q, err := kube.ParseQuantity(v)
	if err != nil || q.Sign() <= 0 {
		return nil, false
	}
	return q, true
}

// tieRank orders the resources that give the same Fits, for a LimitedBy that
// does not depend on map order: CPU, memory, limits, then the counts.
func tieRank(resource string) string {
	switch resource {
	case "requests.cpu", "cpu":
		return "0" + resource
	case "requests.memory", "memory":
		return "1" + resource
	case "limits.cpu", "limits.memory":
		return "2" + resource
	}
	return "3" + resource
}

// room is one quota resource: its enforced ceiling and what is free of it.
type room struct {
	hard, free *big.Rat
}

// capacityOf counts how many more workers the quotas admit. Every quota is
// counted, scoped or not (a scope could exclude the workers, so this may
// undercount, never overcount). Without a quota that limits something a
// worker takes, or with one the quota controller has not filled in yet, the
// capacity is unknown (false).
func capacityOf(quotas []kube.ResourceQuota, now time.Time) (Capacity, bool) {
	var (
		fits      *big.Int
		limitedBy string
		cpu, mem  *room
	)
	for _, q := range quotas {
		for resource, hardS := range q.Status.Hard {
			per, ok := perWorker(resource)
			if !ok {
				continue
			}
			usedS, ok := q.Status.Used[resource]
			if !ok {
				return Capacity{}, false // not computed yet
			}
			hard, herr := kube.ParseQuantity(hardS)
			used, uerr := kube.ParseQuantity(usedS)
			if herr != nil || uerr != nil {
				return Capacity{}, false
			}
			free := new(big.Rat).Sub(hard, used)
			if free.Sign() < 0 {
				free.SetInt64(0)
			}
			n := new(big.Rat).Quo(free, per)
			whole := new(big.Int).Quo(n.Num(), n.Denom()) // free >= 0: truncation is the floor
			if fits == nil || whole.Cmp(fits) < 0 || whole.Cmp(fits) == 0 && tieRank(resource) < tieRank(limitedBy) {
				fits, limitedBy = whole, resource
			}
			r := &room{hard: hard, free: free}
			switch resource {
			case "requests.cpu", "cpu":
				if cpu == nil || free.Cmp(cpu.free) < 0 {
					cpu = r
				}
			case "requests.memory", "memory":
				if mem == nil || free.Cmp(mem.free) < 0 {
					mem = r
				}
			}
		}
	}
	if fits == nil {
		return Capacity{}, false
	}
	c := Capacity{Fits: int(min(fits.Int64(), math.MaxInt32)), LimitedBy: limitedBy, Seen: now}
	if q, ok := perWorker("requests.cpu"); ok {
		c.WorkerCPUCores = cores(q)
	}
	if q, ok := perWorker("requests.memory"); ok {
		c.WorkerMemoryBytes = byteCount(q)
	}
	if cpu != nil {
		hard, free := cores(cpu.hard), cores(cpu.free)
		c.CPUHardCores, c.CPUFreeCores = &hard, &free
	}
	if mem != nil {
		hard, free := byteCount(mem.hard), byteCount(mem.free)
		c.MemoryHardBytes, c.MemoryFreeBytes = &hard, &free
	}
	return c, true
}

// usageOf is a pod's metrics as the web shows them.
func usageOf(m kube.PodMetrics) (Usage, error) {
	cpu, mem, err := m.Usage()
	if err != nil {
		return Usage{}, err
	}
	at, _ := time.Parse(time.RFC3339, m.Timestamp)
	return Usage{CPUCores: math.Round(cpu*1000) / 1000, MemoryBytes: mem, MeasuredAt: at}, nil
}

// cores is a CPU quantity in cores, to the millicore.
func cores(q *big.Rat) float64 {
	f, _ := q.Float64()
	return math.Round(f*1000) / 1000
}

// byteCount is a memory quantity in whole bytes.
func byteCount(q *big.Rat) int64 {
	f, _ := q.Float64()
	return int64(math.Floor(f))
}

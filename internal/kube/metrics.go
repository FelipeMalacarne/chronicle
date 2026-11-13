package kube

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"net/url"
)

// PodMetrics is one pod's usage as the metrics API (metrics.k8s.io/v1beta1,
// served by metrics-server) reports it: each container's CPU and working set
// over the last window.
type PodMetrics struct {
	Metadata ObjectMeta `json:"metadata"`
	// Timestamp is RFC 3339, the end of the window.
	Timestamp  string             `json:"timestamp"`
	Window     string             `json:"window,omitempty"`
	Containers []ContainerMetrics `json:"containers"`
}

// ContainerMetrics is one container's usage ("cpu": "403585713n", "memory":
// "856788Ki").
type ContainerMetrics struct {
	Name  string            `json:"name"`
	Usage map[string]string `json:"usage"`
}

// Usage sums the pod's containers: CPU in cores, memory in bytes.
func (m PodMetrics) Usage() (cpuCores float64, memoryBytes int64, err error) {
	cpu, mem := new(big.Rat), new(big.Rat)
	add := func(sum *big.Rat, c ContainerMetrics, name string) error {
		v, ok := c.Usage[name]
		if !ok {
			return nil
		}
		q, qerr := ParseQuantity(v)
		if qerr != nil {
			return fmt.Errorf("pod %s container %s %s: %w", m.Metadata.Name, c.Name, name, qerr)
		}
		sum.Add(sum, q)
		return nil
	}
	for _, c := range m.Containers {
		if cerr := add(cpu, c, "cpu"); cerr != nil {
			return 0, 0, cerr
		}
		if merr := add(mem, c, "memory"); merr != nil {
			return 0, 0, merr
		}
	}
	cpuCores, _ = cpu.Float64()
	memF, _ := mem.Float64()
	return cpuCores, int64(math.Round(memF)), nil
}

// ListPodMetrics lists the usage of the namespace's pods that match a label
// selector. Without metrics-server the API answers 404 (ErrNotFound) or 503;
// without the Role's rule, 403.
func (c *Client) ListPodMetrics(ctx context.Context, selector string) ([]PodMetrics, error) {
	var out struct {
		Items []PodMetrics `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, "/apis/metrics.k8s.io/v1beta1/namespaces/"+url.PathEscape(c.Namespace)+"/pods",
		url.Values{"labelSelector": {selector}}, "", nil, &out)
	return out.Items, err
}

// ResourceQuota is v1 ResourceQuota: the namespace's ceilings ("requests.cpu":
// "2200m", "count/deployments.apps": "4", ...) and what is used of them.
type ResourceQuota struct {
	Metadata ObjectMeta          `json:"metadata"`
	Spec     ResourceQuotaSpec   `json:"spec"`
	Status   ResourceQuotaStatus `json:"status"`
}

// ResourceQuotaSpec is what the quota asks for.
type ResourceQuotaSpec struct {
	Hard   map[string]string `json:"hard,omitempty"`
	Scopes []string          `json:"scopes,omitempty"`
}

// ResourceQuotaStatus is what the quota controller computed: the enforced
// ceilings and the namespace's use of each.
type ResourceQuotaStatus struct {
	Hard map[string]string `json:"hard,omitempty"`
	Used map[string]string `json:"used,omitempty"`
}

// ListResourceQuotas lists the namespace's ResourceQuotas.
func (c *Client) ListResourceQuotas(ctx context.Context) ([]ResourceQuota, error) {
	var out struct {
		Items []ResourceQuota `json:"items"`
	}
	err := c.do(ctx, http.MethodGet, "/api/v1/namespaces/"+url.PathEscape(c.Namespace)+"/resourcequotas", nil, "", nil, &out)
	return out.Items, err
}

package kube

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// The worker pod contract (docs/11 section 13.3). The ValidatingAdmissionPolicy
// in gielinor checks the same rules on every Deployment the server writes.
const (
	LabelName      = "app.kubernetes.io/name"
	LabelComponent = "app.kubernetes.io/component"
	LabelAccount   = "chronicle.ftm.dev.br/account"
	ComponentWork  = "worker"
	AppName        = "chronicle"

	WorkerServiceAccount = "chronicle-worker"
	WorkerAudience       = "chronicle"
	CacheClaim           = "microbot-game-cache"

	workerHome      = "/home/worker"
	workerDevHome   = workerHome + "/microbot-dev"
	workerCacheDir  = workerDevHome + "/.runelite/jagexcache"
	workerSecrets   = "/run/chronicle"
	workerTokenDir  = "/var/run/secrets/chronicle"
	workerTokenFile = workerTokenDir + "/token"
	workerUID       = int64(65532)
)

// WorkerSelector selects every worker Deployment and pod.
const WorkerSelector = LabelName + "=" + AppName + "," + LabelComponent + "=" + ComponentWork

// WorkerName is the Deployment of an account's worker: worker-<account>, in
// lower case, with anything outside [a-z0-9-] as '-', and short enough for
// the pod names derived from it (acc_iron_man -> worker-acc-iron-man).
func WorkerName(account string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(account) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 40 {
		name = strings.TrimRight(name[:40], "-")
	}
	return "worker-" + name
}

// WorkerSpec is what varies between workers.
type WorkerSpec struct {
	Account string
	Image   string
	// ServerURL is CHRONICLE_URL of the agent in the pod: the planner's
	// in-cluster Service.
	ServerURL string
	// Version is the planner build that wrote the Deployment (an annotation:
	// the web shows "versão antiga" when it differs from the server's).
	Version string
}

// WorkerDeployment builds the Deployment of one worker.
func WorkerDeployment(namespace string, s WorkerSpec) Deployment {
	name := WorkerName(s.Account)
	labels := map[string]string{LabelName: AppName, LabelComponent: ComponentWork, LabelAccount: s.Account}
	one, history := int32(1), int32(1)
	no, yes := false, true
	uid, grace := workerUID, int64(60)
	return Deployment{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Metadata: ObjectMeta{
			Name: name, Namespace: namespace, Labels: labels,
			Annotations: map[string]string{"chronicle.ftm.dev.br/version": s.Version},
		},
		Spec: DeploymentSpec{
			Replicas:             &one,
			RevisionHistoryLimit: &history,
			// Never two clients on one account: Recreate stops the old pod
			// before the new one starts (docs/11 section 6.1).
			Strategy: DeploymentStrategy{Type: "Recreate"},
			Selector: LabelSelector{MatchLabels: labels},
			Template: PodTemplate{
				Metadata: ObjectMeta{Labels: labels},
				Spec: PodSpec{
					ServiceAccountName:            WorkerServiceAccount,
					AutomountServiceAccountToken:  &no,
					EnableServiceLinks:            &no,
					TerminationGracePeriodSeconds: &grace,
					SecurityContext: &PodSecurityContext{
						RunAsUser: &uid, RunAsGroup: &uid, FSGroup: &uid, RunAsNonRoot: &yes,
						SeccompProfile: &SeccompProfile{Type: "RuntimeDefault"},
					},
					Containers: []Container{{
						Name:            "worker",
						Image:           s.Image,
						ImagePullPolicy: "IfNotPresent",
						Env: []EnvVar{
							{Name: "WORKER_ACCOUNT", Value: s.Account},
							{Name: "CHRONICLE_URL", Value: s.ServerURL},
							{Name: "CHRONICLE_AGENT_TOKEN_FILE", Value: workerTokenFile},
							{Name: "HOME", Value: workerHome},
							{Name: "MB_DEV_HOME", Value: workerDevHome},
							{Name: "MB_XMX", Value: "1024m"},
							{Name: "MB_HWID_STORE_DIR", Value: workerSecrets},
						},
						Resources: ResourceRequirements{
							Requests: map[string]string{"cpu": "500m", "memory": "1Gi"},
							Limits:   map[string]string{"cpu": "1", "memory": "1792Mi"},
						},
						VolumeMounts: []VolumeMount{
							{Name: "home", MountPath: workerHome},
							{Name: "secrets", MountPath: workerSecrets},
							{Name: "game-cache", MountPath: workerCacheDir},
							{Name: "chronicle-token", MountPath: workerTokenDir, ReadOnly: true},
						},
						SecurityContext: &ContainerSecurityContext{
							AllowPrivilegeEscalation: &no, Privileged: &no,
							Capabilities: &Capabilities{Drop: []string{"ALL"}},
						},
					}},
					Volumes: []Volume{
						{Name: "home", EmptyDir: &EmptyDirSource{}},
						{Name: "secrets", EmptyDir: &EmptyDirSource{Medium: "Memory", SizeLimit: "1Mi"}},
						{Name: "game-cache", PersistentVolumeClaim: &PVCSource{ClaimName: CacheClaim}},
						{Name: "chronicle-token", Projected: &ProjectedVolumeSource{Sources: []VolumeProjection{{
							ServiceAccountToken: &ServiceAccountTokenProjection{Audience: WorkerAudience, ExpirationSeconds: 3600, Path: "token"},
						}}}},
					},
				},
			},
		},
	}
}

// Pod phases the web shows (docs/11 section 2.1, "subindo, passo a passo").
const (
	PodPending     = "pending"     // not scheduled: no room on the node
	PodBlocked     = "blocked"     // no pod at all: the ReplicaSet could not create it (over quota)
	PodPulling     = "pulling"     // scheduled, container not up yet
	PodStarting    = "starting"    // container up, agent not linked yet
	PodRunning     = "running"     // container ready
	PodFailing     = "failing"     // crash loop or image error
	PodTerminating = "terminating" // being deleted
)

// PodState summarizes a worker pod for the web, with the reason in words of
// the API (Unschedulable, ImagePullBackOff, CrashLoopBackOff, ...).
func PodState(p Pod) (state, reason string) {
	if p.Metadata.DeletionTimestamp != "" {
		return PodTerminating, ""
	}
	for _, c := range p.Status.Conditions {
		if c.Type == "PodScheduled" && c.Status == "False" {
			return PodPending, firstNonEmpty(c.Reason, c.Message)
		}
	}
	for _, cs := range p.Status.ContainerStatuses {
		if w := cs.State.Waiting; w != nil {
			switch w.Reason {
			case "ImagePullBackOff", "ErrImagePull", "CrashLoopBackOff", "CreateContainerConfigError", "InvalidImageName":
				return PodFailing, w.Reason
			}
			return PodPulling, w.Reason
		}
		if cs.Ready {
			return PodRunning, ""
		}
		if cs.State.Running != nil {
			return PodStarting, ""
		}
		if t := cs.State.Terminated; t != nil {
			return PodFailing, t.Reason
		}
	}
	if p.Status.Phase == "Pending" {
		return PodPulling, p.Status.Reason
	}
	return PodStarting, p.Status.Reason
}

// DeploymentBlocked reports why a Deployment has no pod when its ReplicaSet
// could not create one (ReplicaFailure, e.g. exceeded quota).
func DeploymentBlocked(d Deployment) (string, bool) {
	for _, c := range d.Status.Conditions {
		if c.Type == "ReplicaFailure" && c.Status == "True" {
			return firstNonEmpty(c.Message, c.Reason), true
		}
	}
	return "", false
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// WorkerIdentity is who presented a worker pod's projected token.
type WorkerIdentity struct {
	Pod     string
	Account string
}

// ErrNotWorker: the token is valid but not a worker pod's.
var ErrNotWorker = errors.New("kube: not a worker pod's token")

// VerifyWorkerToken checks a projected token by TokenReview (audience
// "chronicle") and accepts it only from a live pod of the chronicle-worker
// service account in this namespace whose labels make it a worker; the
// account is the pod's label, never what the agent claims.
func (c *Client) VerifyWorkerToken(ctx context.Context, token string) (WorkerIdentity, error) {
	st, err := c.TokenReview(ctx, token, []string{WorkerAudience})
	if err != nil {
		return WorkerIdentity{}, err
	}
	if !st.Authenticated {
		return WorkerIdentity{}, fmt.Errorf("kube: token not authenticated: %s", st.Error)
	}
	want := "system:serviceaccount:" + c.Namespace + ":" + WorkerServiceAccount
	if st.User.Username != want {
		return WorkerIdentity{}, fmt.Errorf("%w: user %q", ErrNotWorker, st.User.Username)
	}
	pod := firstOf(st.User.Extra["authentication.kubernetes.io/pod-name"])
	podUID := firstOf(st.User.Extra["authentication.kubernetes.io/pod-uid"])
	if pod == "" {
		return WorkerIdentity{}, fmt.Errorf("%w: token not bound to a pod", ErrNotWorker)
	}
	p, err := c.GetPod(ctx, pod)
	if err != nil {
		return WorkerIdentity{}, fmt.Errorf("kube: worker pod %s: %w", pod, err)
	}
	if podUID != "" && p.Metadata.UID != podUID {
		return WorkerIdentity{}, fmt.Errorf("%w: pod %s was replaced", ErrNotWorker, pod)
	}
	if p.Metadata.Labels[LabelComponent] != ComponentWork || p.Metadata.Labels[LabelName] != AppName ||
		p.Spec.ServiceAccountName != WorkerServiceAccount {
		return WorkerIdentity{}, fmt.Errorf("%w: pod %s is not a worker", ErrNotWorker, pod)
	}
	account := p.Metadata.Labels[LabelAccount]
	if account == "" {
		return WorkerIdentity{}, fmt.Errorf("%w: pod %s has no account label", ErrNotWorker, pod)
	}
	return WorkerIdentity{Pod: pod, Account: account}, nil
}

func firstOf(xs []string) string {
	if len(xs) == 0 {
		return ""
	}
	return xs[0]
}

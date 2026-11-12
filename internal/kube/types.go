package kube

// The few API objects the workers need, with only the fields Chronicle sets
// or reads. Server-side apply only touches the fields sent, so a missing
// field here is a field Chronicle does not own.

// ObjectMeta is the metadata of an object.
type ObjectMeta struct {
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	UID         string            `json:"uid,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	// CreationTimestamp is RFC 3339, as the API sends it.
	CreationTimestamp string `json:"creationTimestamp,omitempty"`
	DeletionTimestamp string `json:"deletionTimestamp,omitempty"`
}

// Deployment is apps/v1 Deployment.
type Deployment struct {
	APIVersion string           `json:"apiVersion,omitempty"`
	Kind       string           `json:"kind,omitempty"`
	Metadata   ObjectMeta       `json:"metadata"`
	Spec       DeploymentSpec   `json:"spec"`
	Status     DeploymentStatus `json:"status,omitzero"`
}

// DeploymentSpec is the part of the spec Chronicle sets.
type DeploymentSpec struct {
	Replicas             *int32             `json:"replicas,omitempty"`
	RevisionHistoryLimit *int32             `json:"revisionHistoryLimit,omitempty"`
	Strategy             DeploymentStrategy `json:"strategy"`
	Selector             LabelSelector      `json:"selector"`
	Template             PodTemplate        `json:"template"`
}

// DeploymentStrategy is the rollout strategy.
type DeploymentStrategy struct {
	Type string `json:"type"`
}

// LabelSelector selects by labels.
type LabelSelector struct {
	MatchLabels map[string]string `json:"matchLabels"`
}

// DeploymentStatus is what the controller reports.
type DeploymentStatus struct {
	Replicas      int32 `json:"replicas,omitempty"`
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
}

// PodTemplate is a Deployment's pod template.
type PodTemplate struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     PodSpec    `json:"spec"`
}

// PodSpec is the part of a pod's spec Chronicle sets or reads.
type PodSpec struct {
	ServiceAccountName            string              `json:"serviceAccountName,omitempty"`
	AutomountServiceAccountToken  *bool               `json:"automountServiceAccountToken,omitempty"`
	TerminationGracePeriodSeconds *int64              `json:"terminationGracePeriodSeconds,omitempty"`
	EnableServiceLinks            *bool               `json:"enableServiceLinks,omitempty"`
	SecurityContext               *PodSecurityContext `json:"securityContext,omitempty"`
	Containers                    []Container         `json:"containers"`
	Volumes                       []Volume            `json:"volumes,omitempty"`
	NodeName                      string              `json:"nodeName,omitempty"`
}

// PodSecurityContext is the pod-level security context.
type PodSecurityContext struct {
	RunAsUser      *int64          `json:"runAsUser,omitempty"`
	RunAsGroup     *int64          `json:"runAsGroup,omitempty"`
	FSGroup        *int64          `json:"fsGroup,omitempty"`
	RunAsNonRoot   *bool           `json:"runAsNonRoot,omitempty"`
	SeccompProfile *SeccompProfile `json:"seccompProfile,omitempty"`
}

// SeccompProfile names a seccomp profile type.
type SeccompProfile struct {
	Type string `json:"type"`
}

// Container is one container.
type Container struct {
	Name            string                    `json:"name"`
	Image           string                    `json:"image"`
	ImagePullPolicy string                    `json:"imagePullPolicy,omitempty"`
	Env             []EnvVar                  `json:"env,omitempty"`
	Resources       ResourceRequirements      `json:"resources,omitzero"`
	VolumeMounts    []VolumeMount             `json:"volumeMounts,omitempty"`
	SecurityContext *ContainerSecurityContext `json:"securityContext,omitempty"`
}

// EnvVar is one environment variable with a literal value.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ResourceRequirements are requests and limits ("500m", "1Gi").
type ResourceRequirements struct {
	Requests map[string]string `json:"requests,omitempty"`
	Limits   map[string]string `json:"limits,omitempty"`
}

// VolumeMount mounts a volume in a container.
type VolumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly,omitempty"`
}

// ContainerSecurityContext is the container-level security context.
type ContainerSecurityContext struct {
	AllowPrivilegeEscalation *bool         `json:"allowPrivilegeEscalation,omitempty"`
	Privileged               *bool         `json:"privileged,omitempty"`
	Capabilities             *Capabilities `json:"capabilities,omitempty"`
}

// Capabilities adds and drops Linux capabilities.
type Capabilities struct {
	Drop []string `json:"drop,omitempty"`
}

// Volume is one pod volume; exactly one source is set.
type Volume struct {
	Name                  string                 `json:"name"`
	EmptyDir              *EmptyDirSource        `json:"emptyDir,omitempty"`
	PersistentVolumeClaim *PVCSource             `json:"persistentVolumeClaim,omitempty"`
	Projected             *ProjectedVolumeSource `json:"projected,omitempty"`
}

// EmptyDirSource is an emptyDir; Medium "Memory" is a tmpfs.
type EmptyDirSource struct {
	Medium    string `json:"medium,omitempty"`
	SizeLimit string `json:"sizeLimit,omitempty"`
}

// PVCSource mounts a claim.
type PVCSource struct {
	ClaimName string `json:"claimName"`
}

// ProjectedVolumeSource projects tokens into a volume.
type ProjectedVolumeSource struct {
	Sources []VolumeProjection `json:"sources"`
}

// VolumeProjection is one projected source.
type VolumeProjection struct {
	ServiceAccountToken *ServiceAccountTokenProjection `json:"serviceAccountToken,omitempty"`
}

// ServiceAccountTokenProjection is a bound token for one audience.
type ServiceAccountTokenProjection struct {
	Audience          string `json:"audience"`
	ExpirationSeconds int64  `json:"expirationSeconds"`
	Path              string `json:"path"`
}

// Pod is a pod as read from the API.
type Pod struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     PodSpec    `json:"spec"`
	Status   PodStatus  `json:"status"`
}

// PodStatus is the pod's observed state.
type PodStatus struct {
	Phase             string            `json:"phase"`
	Reason            string            `json:"reason,omitempty"`
	Message           string            `json:"message,omitempty"`
	StartTime         string            `json:"startTime,omitempty"`
	Conditions        []PodCondition    `json:"conditions,omitempty"`
	ContainerStatuses []ContainerStatus `json:"containerStatuses,omitempty"`
}

// PodCondition is one condition (PodScheduled, Ready, ...).
type PodCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

// ContainerStatus is one container's state.
type ContainerStatus struct {
	Name         string         `json:"name"`
	Image        string         `json:"image"`
	Ready        bool           `json:"ready"`
	RestartCount int32          `json:"restartCount"`
	State        ContainerState `json:"state"`
}

// ContainerState tells waiting, running or terminated apart.
type ContainerState struct {
	Waiting *struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	} `json:"waiting,omitempty"`
	Running *struct {
		StartedAt string `json:"startedAt"`
	} `json:"running,omitempty"`
	Terminated *struct {
		Reason   string `json:"reason"`
		ExitCode int32  `json:"exitCode"`
	} `json:"terminated,omitempty"`
}

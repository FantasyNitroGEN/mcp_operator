package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RegistrySource defines the source configuration for the registry
type RegistrySource struct {
	// Type specifies the source type (github, git, oci)
	Type string `json:"type"`

	// Github contains GitHub-specific source configuration
	Github *GithubSource `json:"github,omitempty"`

	// Path specifies the root path within the repository containing server folders
	Path string `json:"path,omitempty"`
}

// GithubSource contains GitHub-specific source configuration
type GithubSource struct {
	// Repo specifies the GitHub repository in owner/repo format
	Repo string `json:"repo"`

	// Branch specifies the branch or ref to use (defaults to main)
	Branch string `json:"branch,omitempty"`
}

// RegistryFilters contains include/exclude patterns for servers
type RegistryFilters struct {
	// Include contains patterns of servers to include
	Include []string `json:"include,omitempty"`

	// Exclude contains patterns of servers to exclude
	Exclude []string `json:"exclude,omitempty"`
}

// RegistryServer contains information about a server available in the registry
type RegistryServer struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Path        string   `json:"path,omitempty"`
}

// MCPRegistrySpec defines the desired state of MCPRegistry
type MCPRegistrySpec struct {
	// URL is the base URL of the MCP registry
	URL string `json:"url,omitempty"`

	// Type specifies the registry type (github, local, etc.)
	Type string `json:"type,omitempty"`

	// Source contains the source configuration for the registry
	Source *RegistrySource `json:"source,omitempty"`

	// Auth contains authentication information for the registry
	Auth *RegistryAuth `json:"auth,omitempty"`

	// SyncInterval specifies how often to sync with the registry
	SyncInterval *metav1.Duration `json:"syncInterval,omitempty"`

	// RefreshInterval specifies how often to refresh the registry
	RefreshInterval *metav1.Duration `json:"refreshInterval,omitempty"`

	// Servers is a list of specific servers to sync from this registry
	Servers []string `json:"servers,omitempty"`

	// Filters contains include/exclude patterns for servers
	Filters *RegistryFilters `json:"filters,omitempty"`
}

// RegistryAuth contains authentication information
type RegistryAuth struct {
	// SecretRef references a secret containing authentication credentials
	SecretRef *SecretReference `json:"secretRef,omitempty"`

	// Token is a direct token for authentication (not recommended for production)
	Token string `json:"token,omitempty"`
}

// MCPRegistryStatus defines the observed state of MCPRegistry
type MCPRegistryStatus struct {
	// Phase represents the current phase of the registry
	Phase MCPRegistryPhase `json:"phase,omitempty"`

	// Message provides human-readable information about the registry status
	Message string `json:"message,omitempty"`

	// Reason provides a brief reason for the current status
	Reason string `json:"reason,omitempty"`

	// ObservedRevision contains the SHA/etag of the last observed revision
	ObservedRevision string `json:"observedRevision,omitempty"`

	// LastSyncTime is the last time the registry was successfully synced
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// ObservedGeneration reflects the generation of the most recently observed MCPRegistry
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// ServersDiscovered is the number of servers discovered in this registry
	ServersDiscovered int32 `json:"serversDiscovered,omitempty"`

	// AvailableServers is the number of servers available in this registry (deprecated, use ServersDiscovered)
	AvailableServers int32 `json:"availableServers,omitempty"`

	// Errors contains the last N errors as strings
	Errors []string `json:"errors,omitempty"`

	// RateLimitRemaining contains the remaining GitHub API rate limit (if applicable)
	RateLimitRemaining *int32 `json:"rateLimitRemaining,omitempty"`

	// Conditions represent the latest available observations of the registry's state
	Conditions []MCPRegistryCondition `json:"conditions,omitempty"`

	// Servers contains information about available servers in the registry
	Servers []RegistryServer `json:"servers,omitempty"`

	// ServerList contains information about available servers in the registry (deprecated, use Servers)
	ServerList []MCPServerInfo `json:"serverList,omitempty"`
}

// MCPRegistryPhase represents the phase of the registry lifecycle
type MCPRegistryPhase string

const (
	// MCPRegistryPhasePending registry is being initialized
	MCPRegistryPhasePending MCPRegistryPhase = "Pending"

	// MCPRegistryPhaseReady registry is ready and synced
	MCPRegistryPhaseReady MCPRegistryPhase = "Ready"

	// MCPRegistryPhaseFailed registry is in a failed state
	MCPRegistryPhaseFailed MCPRegistryPhase = "Failed"

	// MCPRegistryPhaseSyncing registry is currently syncing
	MCPRegistryPhaseSyncing MCPRegistryPhase = "Syncing"
)

// MCPRegistryCondition describes the state of a registry at a certain point
type MCPRegistryCondition struct {
	// Type of registry condition
	Type MCPRegistryConditionType `json:"type"`

	// Status of the condition, one of True, False, Unknown
	Status metav1.ConditionStatus `json:"status"`

	// Last time the condition transitioned from one status to another
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// Unique, one-word, CamelCase reason for the condition's last transition
	Reason string `json:"reason"`

	// Human-readable message indicating details about last transition
	Message string `json:"message"`
}

// MCPRegistryConditionType represents the type of registry condition
type MCPRegistryConditionType string

const (
	// MCPRegistryConditionReady indicates whether the registry is ready
	MCPRegistryConditionReady MCPRegistryConditionType = "Ready"

	// MCPRegistryConditionSynced indicates whether the registry has been synced
	MCPRegistryConditionSynced MCPRegistryConditionType = "Synced"

	// MCPRegistryConditionReachable indicates whether the registry is reachable
	MCPRegistryConditionReachable MCPRegistryConditionType = "Reachable"

	// MCPRegistryConditionSyncing indicates whether the registry is currently syncing
	MCPRegistryConditionSyncing MCPRegistryConditionType = "Syncing"

	// MCPRegistryConditionRateLimited indicates whether the registry is rate limited
	MCPRegistryConditionRateLimited MCPRegistryConditionType = "RateLimited"

	// MCPRegistryConditionAuthFailed indicates whether authentication has failed
	MCPRegistryConditionAuthFailed MCPRegistryConditionType = "AuthFailed"

	// MCPRegistryConditionAuthenticated indicates whether authentication is successful
	MCPRegistryConditionAuthenticated MCPRegistryConditionType = "Authenticated"
)

// MCPServerInfo contains information about a server available in the registry
type MCPServerInfo struct {
	// Name of the server
	Name string `json:"name"`

	// Version of the server
	Version string `json:"version,omitempty"`

	// Description of the server
	Description string `json:"description,omitempty"`

	// LastUpdated is when the server was last updated in the registry
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Servers",type=integer,JSONPath=".status.serversDiscovered"
// +kubebuilder:printcolumn:name="LastSync",type=date,JSONPath=".status.lastSyncTime"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:validation:XValidation:rule="has(self.spec.url) || (has(self.spec.source) && has(self.spec.source.github) && self.spec.source.github.repo != '')",message="Either spec.url or spec.source.github.repo must be set"
// +kubebuilder:validation:XValidation:rule="!(has(self.spec.url) && has(self.spec.source) && has(self.spec.source.github) && self.spec.source.github.repo != '')",message="Specify only one of spec.url or spec.source.github.repo"

// MCPRegistry is the Schema for the mcpregistries API
type MCPRegistry struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPRegistrySpec   `json:"spec,omitempty"`
	Status MCPRegistryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MCPRegistryList contains a list of MCPRegistry
type MCPRegistryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPRegistry `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPRegistry{}, &MCPRegistryList{})
}

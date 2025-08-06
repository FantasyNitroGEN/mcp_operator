package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MCPServerBackupSpec defines the desired state of MCPServerBackup
type MCPServerBackupSpec struct {
	// MCPServerRef references the MCPServer to backup
	MCPServerRef string `json:"mcpServerRef"`

	// Schedule defines when to create backups (cron format)
	Schedule string `json:"schedule,omitempty"`

	// RetentionPolicy defines how long to keep backups
	RetentionPolicy RetentionPolicy `json:"retentionPolicy,omitempty"`

	// BackupType defines the type of backup (Full, Incremental)
	BackupType BackupType `json:"backupType,omitempty"`

	// StorageLocation defines where to store the backup
	StorageLocation StorageLocation `json:"storageLocation,omitempty"`

	// Suspend indicates whether the backup schedule is suspended
	Suspend *bool `json:"suspend,omitempty"`
}

// MCPServerBackupStatus defines the observed state of MCPServerBackup
type MCPServerBackupStatus struct {
	// Phase represents the current phase of the backup
	Phase BackupPhase `json:"phase,omitempty"`

	// Message provides human-readable information about the backup status
	Message string `json:"message,omitempty"`

	// Reason provides the reason for the current status
	Reason string `json:"reason,omitempty"`

	// StartTime is when the backup started
	StartTime *metav1.Time `json:"startTime,omitempty"`

	// CompletionTime is when the backup completed
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// BackupSize is the size of the backup in bytes
	BackupSize int64 `json:"backupSize,omitempty"`

	// BackupLocation is the actual location where the backup is stored
	BackupLocation string `json:"backupLocation,omitempty"`

	// LastScheduleTime is the last time a scheduled backup was created
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`

	// NextScheduleTime is the next time a scheduled backup will be created
	NextScheduleTime *metav1.Time `json:"nextScheduleTime,omitempty"`

	// BackupData contains the actual backup data (for small backups)
	BackupData *MCPServerBackupData `json:"backupData,omitempty"`

	// Conditions represent the latest available observations of the backup's state
	Conditions []MCPServerBackupCondition `json:"conditions,omitempty"`
}

// MCPServerBackupData contains the actual backup data
type MCPServerBackupData struct {
	// MCPServerSpec is the backed up MCPServer specification
	MCPServerSpec MCPServerSpec `json:"mcpServerSpec"`

	// Metadata contains additional metadata about the backup
	Metadata BackupMetadata `json:"metadata"`

	// Dependencies contains information about related resources
	Dependencies []ResourceReference `json:"dependencies,omitempty"`
}

// BackupMetadata contains metadata about the backup
type BackupMetadata struct {
	// BackupVersion is the version of the backup format
	BackupVersion string `json:"backupVersion"`

	// OperatorVersion is the version of the operator that created the backup
	OperatorVersion string `json:"operatorVersion"`

	// KubernetesVersion is the version of Kubernetes when backup was created
	KubernetesVersion string `json:"kubernetesVersion"`

	// CreatedBy indicates who/what created the backup
	CreatedBy string `json:"createdBy"`

	// Labels are additional labels for the backup
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are additional annotations for the backup
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ResourceReference represents a reference to a related Kubernetes resource
type ResourceReference struct {
	// APIVersion of the referenced resource
	APIVersion string `json:"apiVersion"`

	// Kind of the referenced resource
	Kind string `json:"kind"`

	// Name of the referenced resource
	Name string `json:"name"`

	// Namespace of the referenced resource
	Namespace string `json:"namespace,omitempty"`

	// UID of the referenced resource
	UID string `json:"uid,omitempty"`
}

// RetentionPolicy defines backup retention settings
type RetentionPolicy struct {
	// KeepLast specifies how many recent backups to keep
	KeepLast *int32 `json:"keepLast,omitempty"`

	// KeepDaily specifies how many daily backups to keep
	KeepDaily *int32 `json:"keepDaily,omitempty"`

	// KeepWeekly specifies how many weekly backups to keep
	KeepWeekly *int32 `json:"keepWeekly,omitempty"`

	// KeepMonthly specifies how many monthly backups to keep
	KeepMonthly *int32 `json:"keepMonthly,omitempty"`

	// MaxAge specifies the maximum age of backups to keep
	MaxAge *metav1.Duration `json:"maxAge,omitempty"`
}

// StorageLocation defines where backups are stored
type StorageLocation struct {
	// Type of storage (Local, S3, GCS, Azure)
	Type StorageType `json:"type"`

	// Local storage configuration
	Local *LocalStorage `json:"local,omitempty"`

	// S3 storage configuration
	S3 *S3Storage `json:"s3,omitempty"`

	// GCS storage configuration
	GCS *GCSStorage `json:"gcs,omitempty"`

	// Azure storage configuration
	Azure *AzureStorage `json:"azure,omitempty"`
}

// LocalStorage defines local storage configuration
type LocalStorage struct {
	// Path where backups are stored locally
	Path string `json:"path"`

	// VolumeSource for the storage volume
	VolumeSource *VolumeSource `json:"volumeSource,omitempty"`
}

// VolumeSource represents a volume source for backup storage
type VolumeSource struct {
	// PersistentVolumeClaim represents a reference to a PersistentVolumeClaim
	PersistentVolumeClaim *PVCVolumeSource `json:"persistentVolumeClaim,omitempty"`

	// HostPath represents a pre-existing file or directory on the host machine
	HostPath *HostPathVolumeSource `json:"hostPath,omitempty"`

	// EmptyDir represents a temporary directory that shares a pod's lifetime
	EmptyDir *EmptyDirVolumeSource `json:"emptyDir,omitempty"`
}

// PVCVolumeSource represents a PVC volume source
type PVCVolumeSource struct {
	// ClaimName is the name of the PVC
	ClaimName string `json:"claimName"`

	// ReadOnly specifies whether the volume is read-only
	ReadOnly bool `json:"readOnly,omitempty"`
}

// HostPathVolumeSource represents a host path volume source
type HostPathVolumeSource struct {
	// Path of the directory on the host
	Path string `json:"path"`

	// Type of the host path
	Type *HostPathType `json:"type,omitempty"`
}

// EmptyDirVolumeSource represents an empty directory volume source
type EmptyDirVolumeSource struct {
	// Medium represents what type of storage medium should back this directory
	Medium StorageMedium `json:"medium,omitempty"`

	// SizeLimit is the total amount of local storage required for this EmptyDir volume
	SizeLimit *string `json:"sizeLimit,omitempty"`
}

// S3Storage defines S3 storage configuration
type S3Storage struct {
	// Bucket name
	Bucket string `json:"bucket"`

	// Region of the S3 bucket
	Region string `json:"region"`

	// Prefix for backup objects
	Prefix string `json:"prefix,omitempty"`

	// Endpoint for S3-compatible storage
	Endpoint string `json:"endpoint,omitempty"`

	// CredentialsSecret contains AWS credentials
	CredentialsSecret string `json:"credentialsSecret"`
}

// GCSStorage defines Google Cloud Storage configuration
type GCSStorage struct {
	// Bucket name
	Bucket string `json:"bucket"`

	// Prefix for backup objects
	Prefix string `json:"prefix,omitempty"`

	// CredentialsSecret contains GCS credentials
	CredentialsSecret string `json:"credentialsSecret"`
}

// AzureStorage defines Azure Blob Storage configuration
type AzureStorage struct {
	// Container name
	Container string `json:"container"`

	// StorageAccount name
	StorageAccount string `json:"storageAccount"`

	// Prefix for backup objects
	Prefix string `json:"prefix,omitempty"`

	// CredentialsSecret contains Azure credentials
	CredentialsSecret string `json:"credentialsSecret"`
}

// MCPServerBackupCondition describes the state of a backup at a certain point
type MCPServerBackupCondition struct {
	// Type of backup condition
	Type BackupConditionType `json:"type"`

	// Status of the condition (True, False, Unknown)
	Status metav1.ConditionStatus `json:"status"`

	// LastTransitionTime is the last time the condition transitioned
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// Reason is a unique, one-word, CamelCase reason for the condition's last transition
	Reason string `json:"reason"`

	// Message is a human-readable message indicating details about the transition
	Message string `json:"message"`
}

// Enums for backup configuration

// BackupPhase represents the phase of a backup operation
type BackupPhase string

const (
	// BackupPhasePending indicates the backup is pending
	BackupPhasePending BackupPhase = "Pending"
	// BackupPhaseRunning indicates the backup is in progress
	BackupPhaseRunning BackupPhase = "Running"
	// BackupPhaseCompleted indicates the backup completed successfully
	BackupPhaseCompleted BackupPhase = "Completed"
	// BackupPhaseFailed indicates the backup failed
	BackupPhaseFailed BackupPhase = "Failed"
	// BackupPhaseDeleting indicates the backup is being deleted
	BackupPhaseDeleting BackupPhase = "Deleting"
)

// BackupType represents the type of backup
type BackupType string

const (
	// BackupTypeFull represents a full backup
	BackupTypeFull BackupType = "Full"
	// BackupTypeIncremental represents an incremental backup
	BackupTypeIncremental BackupType = "Incremental"
)

// StorageType represents the type of storage for backups
type StorageType string

const (
	// StorageTypeLocal represents local storage
	StorageTypeLocal StorageType = "Local"
	// StorageTypeS3 represents Amazon S3 storage
	StorageTypeS3 StorageType = "S3"
	// StorageTypeGCS represents Google Cloud Storage
	StorageTypeGCS StorageType = "GCS"
	// StorageTypeAzure represents Azure Blob Storage
	StorageTypeAzure StorageType = "Azure"
)

// BackupConditionType represents the type of backup condition
type BackupConditionType string

const (
	// BackupConditionReady indicates the backup is ready
	BackupConditionReady BackupConditionType = "Ready"
	// BackupConditionProgressing indicates the backup is progressing
	BackupConditionProgressing BackupConditionType = "Progressing"
	// BackupConditionFailed indicates the backup has failed
	BackupConditionFailed BackupConditionType = "Failed"
)

// HostPathType represents the type of host path
type HostPathType string

const (
	// HostPathUnset represents an unset host path type
	HostPathUnset HostPathType = ""
	// HostPathDirectoryOrCreate represents a directory that will be created if it doesn't exist
	HostPathDirectoryOrCreate HostPathType = "DirectoryOrCreate"
	// HostPathDirectory represents an existing directory
	HostPathDirectory HostPathType = "Directory"
	// HostPathFileOrCreate represents a file that will be created if it doesn't exist
	HostPathFileOrCreate HostPathType = "FileOrCreate"
	// HostPathFile represents an existing file
	HostPathFile HostPathType = "File"
	// HostPathSocket represents a UNIX socket
	HostPathSocket HostPathType = "Socket"
	// HostPathCharDevice represents a character device
	HostPathCharDevice HostPathType = "CharDevice"
	// HostPathBlockDevice represents a block device
	HostPathBlockDevice HostPathType = "BlockDevice"
)

// StorageMedium represents the storage medium for EmptyDir volumes
type StorageMedium string

const (
	// StorageMediumDefault represents the default storage medium
	StorageMediumDefault StorageMedium = ""
	// StorageMediumMemory represents memory storage
	StorageMediumMemory StorageMedium = "Memory"
	// StorageMediumHugePages represents huge pages storage
	StorageMediumHugePages StorageMedium = "HugePages"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="MCPServer",type="string",JSONPath=".spec.mcpServerRef"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Size",type="string",JSONPath=".status.backupSize"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
//+kubebuilder:printcolumn:name="Schedule",type="string",JSONPath=".spec.schedule"

// MCPServerBackup is the Schema for the mcpserverbackups API
type MCPServerBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MCPServerBackupSpec   `json:"spec,omitempty"`
	Status MCPServerBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MCPServerBackupList contains a list of MCPServerBackup
type MCPServerBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MCPServerBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MCPServerBackup{}, &MCPServerBackupList{})
}

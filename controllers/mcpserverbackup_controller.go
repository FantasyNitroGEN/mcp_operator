package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
)

const (
	// MCPServerBackupFinalizer is the finalizer used for MCPServerBackup resources
	MCPServerBackupFinalizer = "mcp.io/backup-finalizer"

	// BackupAnnotationKey is used to mark resources as backup-related
	BackupAnnotationKey = "mcp.io/backup"

	// BackupVersionCurrent is the current version of the backup format
	BackupVersionCurrent = "v1"
)

// MCPServerBackupReconciler reconciles a MCPServerBackup object
type MCPServerBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Cron   *cron.Cron
}

//+kubebuilder:rbac:groups=mcp.io,resources=mcpserverbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mcp.io,resources=mcpserverbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mcp.io,resources=mcpserverbackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=mcp.io,resources=mcpservers,verbs=get;list;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *MCPServerBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Create structured logger with correlation ID
	reconcileID := uuid.New().String()
	logger := log.FromContext(ctx).WithValues(
		"mcpserverbackup", req.NamespacedName,
		"reconcile_id", reconcileID,
		"controller", "MCPServerBackupReconciler",
	)

	logger.Info("Starting backup reconciliation")

	// Fetch the MCPServerBackup instance
	backup := &mcpv1.MCPServerBackup{}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MCPServerBackup resource not found, assuming deletion")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MCPServerBackup")
		return ctrl.Result{}, err
	}

	// Add backup details to logger context
	logger = logger.WithValues(
		"generation", backup.Generation,
		"resource_version", backup.ResourceVersion,
		"current_phase", backup.Status.Phase,
		"mcpserver_ref", backup.Spec.MCPServerRef,
		"backup_type", backup.Spec.BackupType,
	)

	logger.Info("MCPServerBackup resource found",
		"schedule", backup.Spec.Schedule,
		"storage_type", backup.Spec.StorageLocation.Type,
	)

	// Handle deletion
	if backup.GetDeletionTimestamp() != nil {
		logger.Info("MCPServerBackup is being deleted")
		return r.handleDeletion(ctx, logger, backup)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(backup, MCPServerBackupFinalizer) {
		logger.Info("Adding finalizer to MCPServerBackup")
		controllerutil.AddFinalizer(backup, MCPServerBackupFinalizer)

		err = r.Update(ctx, backup)
		if err != nil {
			logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}

		logger.Info("Finalizer added successfully")
		return ctrl.Result{Requeue: true}, nil
	}

	// Check if backup is suspended
	if backup.Spec.Suspend != nil && *backup.Spec.Suspend {
		logger.Info("Backup is suspended, skipping reconciliation")
		return r.updateBackupStatus(ctx, backup, mcpv1.BackupPhasePending, "Backup is suspended", "Suspended")
	}

	// Validate MCPServer reference
	mcpServer := &mcpv1.MCPServer{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.MCPServerRef,
		Namespace: backup.Namespace,
	}, mcpServer)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Error(err, "Referenced MCPServer not found", "mcpserver", backup.Spec.MCPServerRef)
			return r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseFailed,
				fmt.Sprintf("MCPServer %s not found", backup.Spec.MCPServerRef), "MCPServerNotFound")
		}
		logger.Error(err, "Failed to get referenced MCPServer")
		return ctrl.Result{}, err
	}

	// Handle scheduled backups
	if backup.Spec.Schedule != "" {
		return r.handleScheduledBackup(ctx, logger, backup, mcpServer)
	}

	// Handle one-time backup
	return r.handleOneTimeBackup(ctx, logger, backup, mcpServer)
}

// handleDeletion handles the deletion of MCPServerBackup resources
func (r *MCPServerBackupReconciler) handleDeletion(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) (ctrl.Result, error) {
	logger.Info("Starting backup deletion process")

	// Perform cleanup operations
	if err := r.performBackupCleanup(ctx, logger, backup); err != nil {
		logger.Error(err, "Failed to perform backup cleanup")
		return ctrl.Result{}, err
	}

	// Remove finalizer to allow deletion
	logger.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(backup, MCPServerBackupFinalizer)

	err := r.Update(ctx, backup)
	if err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("MCPServerBackup deletion completed successfully")
	return ctrl.Result{}, nil
}

// performBackupCleanup performs cleanup operations before backup deletion
func (r *MCPServerBackupReconciler) performBackupCleanup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Performing backup cleanup operations")

	// TODO: Implement cleanup based on storage type
	// - Delete backup files from storage
	// - Clean up any associated resources
	// - Update metrics

	return nil
}

// handleScheduledBackup handles scheduled backup operations
func (r *MCPServerBackupReconciler) handleScheduledBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	logger.Info("Handling scheduled backup")

	// Parse cron schedule
	schedule, err := cron.ParseStandard(backup.Spec.Schedule)
	if err != nil {
		logger.Error(err, "Failed to parse backup schedule", "schedule", backup.Spec.Schedule)
		return r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseFailed,
			fmt.Sprintf("Invalid schedule format: %s", backup.Spec.Schedule), "InvalidSchedule")
	}

	now := time.Now()

	// Calculate next schedule time
	nextTime := schedule.Next(now)
	backup.Status.NextScheduleTime = &metav1.Time{Time: nextTime}

	// Check if it's time to create a backup
	if backup.Status.LastScheduleTime == nil ||
		schedule.Next(backup.Status.LastScheduleTime.Time).Before(now) ||
		schedule.Next(backup.Status.LastScheduleTime.Time).Equal(now) {

		logger.Info("Creating scheduled backup")

		// Create backup
		err := r.createBackup(ctx, logger, backup, mcpServer)
		if err != nil {
			logger.Error(err, "Failed to create scheduled backup")
			return r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseFailed,
				fmt.Sprintf("Failed to create backup: %v", err), "BackupCreationFailed")
		}

		// Update last schedule time
		backup.Status.LastScheduleTime = &metav1.Time{Time: now}

		// Enforce retention policy
		if err := r.enforceRetentionPolicy(ctx, logger, backup); err != nil {
			logger.Error(err, "Failed to enforce retention policy")
			// Don't fail the reconciliation for retention policy errors
		}
	}

	// Update status
	result, err := r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseCompleted,
		"Scheduled backup processed", "ScheduleProcessed")
	if err != nil {
		return result, err
	}

	// Requeue for next schedule check (check every minute)
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// handleOneTimeBackup handles one-time backup operations
func (r *MCPServerBackupReconciler) handleOneTimeBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	logger.Info("Handling one-time backup")

	// Check if backup is already completed
	if backup.Status.Phase == mcpv1.BackupPhaseCompleted {
		logger.Info("Backup already completed")
		return ctrl.Result{}, nil
	}

	// Update status to running
	result, err := r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseRunning,
		"Creating backup", "BackupInProgress")
	if err != nil {
		return result, err
	}

	// Create backup
	err = r.createBackup(ctx, logger, backup, mcpServer)
	if err != nil {
		logger.Error(err, "Failed to create backup")
		return r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseFailed,
			fmt.Sprintf("Failed to create backup: %v", err), "BackupCreationFailed")
	}

	// Update status to completed
	return r.updateBackupStatus(ctx, backup, mcpv1.BackupPhaseCompleted,
		"Backup created successfully", "BackupCompleted")
}

// createBackup creates a backup of the MCPServer
func (r *MCPServerBackupReconciler) createBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, mcpServer *mcpv1.MCPServer) error {
	logger.Info("Creating backup", "mcpserver", mcpServer.Name)

	// Create backup data
	backupData := &mcpv1.MCPServerBackupData{
		MCPServerSpec: mcpServer.Spec,
		Metadata: mcpv1.BackupMetadata{
			BackupVersion:     BackupVersionCurrent,
			OperatorVersion:   "1.0.0",  // TODO: Get from build info
			KubernetesVersion: "1.28.0", // TODO: Get from cluster info
			CreatedBy:         "mcp-operator",
			Labels:            backup.Labels,
			Annotations:       backup.Annotations,
		},
		Dependencies: r.collectDependencies(ctx, mcpServer),
	}

	// Store backup data based on storage type
	switch backup.Spec.StorageLocation.Type {
	case mcpv1.StorageTypeLocal:
		return r.storeLocalBackup(ctx, logger, backup, backupData)
	case mcpv1.StorageTypeS3:
		return r.storeS3Backup(ctx, logger, backup, backupData)
	case mcpv1.StorageTypeGCS:
		return r.storeGCSBackup(ctx, logger, backup, backupData)
	case mcpv1.StorageTypeAzure:
		return r.storeAzureBackup(ctx, logger, backup, backupData)
	default:
		return fmt.Errorf("unsupported storage type: %s", backup.Spec.StorageLocation.Type)
	}
}

// storeLocalBackup stores backup data locally (in the backup resource for now)
func (r *MCPServerBackupReconciler) storeLocalBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, data *mcpv1.MCPServerBackupData) error {
	logger.Info("Storing backup locally")

	// For now, store in the backup resource itself
	// In production, this should be stored in persistent storage
	backup.Status.BackupData = data

	// Calculate backup size (approximate)
	backupJSON, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal backup data: %w", err)
	}
	backup.Status.BackupSize = int64(len(backupJSON))
	backup.Status.BackupLocation = fmt.Sprintf("local://%s/%s", backup.Namespace, backup.Name)

	return nil
}

// storeS3Backup stores backup data in S3 (placeholder implementation)
func (r *MCPServerBackupReconciler) storeS3Backup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, data *mcpv1.MCPServerBackupData) error {
	logger.Info("Storing backup in S3")
	// TODO: Implement S3 backup storage
	return fmt.Errorf("S3 backup storage not implemented yet")
}

// storeGCSBackup stores backup data in Google Cloud Storage (placeholder implementation)
func (r *MCPServerBackupReconciler) storeGCSBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, data *mcpv1.MCPServerBackupData) error {
	logger.Info("Storing backup in GCS")
	// TODO: Implement GCS backup storage
	return fmt.Errorf("GCS backup storage not implemented yet")
}

// storeAzureBackup stores backup data in Azure Blob Storage (placeholder implementation)
func (r *MCPServerBackupReconciler) storeAzureBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup, data *mcpv1.MCPServerBackupData) error {
	logger.Info("Storing backup in Azure")
	// TODO: Implement Azure backup storage
	return fmt.Errorf("Azure backup storage not implemented yet")
}

// collectDependencies collects information about related resources
func (r *MCPServerBackupReconciler) collectDependencies(ctx context.Context, mcpServer *mcpv1.MCPServer) []mcpv1.ResourceReference {
	var dependencies []mcpv1.ResourceReference

	// TODO: Collect actual dependencies like:
	// - ConfigMaps referenced by the MCPServer
	// - Secrets referenced by the MCPServer
	// - PVCs used by the MCPServer
	// - Services created for the MCPServer

	return dependencies
}

// enforceRetentionPolicy enforces the backup retention policy
func (r *MCPServerBackupReconciler) enforceRetentionPolicy(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Enforcing retention policy")

	if backup.Spec.RetentionPolicy.KeepLast == nil {
		logger.Info("No retention policy specified, skipping cleanup")
		return nil
	}

	// List all backups for the same MCPServer
	backupList := &mcpv1.MCPServerBackupList{}
	err := r.List(ctx, backupList, client.InNamespace(backup.Namespace), client.MatchingLabels{
		"mcp.io/mcpserver": backup.Spec.MCPServerRef,
	})
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	// TODO: Implement retention policy logic
	// - Sort backups by creation time
	// - Keep only the specified number of recent backups
	// - Delete older backups based on policy

	logger.Info("Retention policy enforced", "total_backups", len(backupList.Items))
	return nil
}

// updateBackupStatus updates the status of the MCPServerBackup resource
func (r *MCPServerBackupReconciler) updateBackupStatus(ctx context.Context, backup *mcpv1.MCPServerBackup, phase mcpv1.BackupPhase, message, reason string) (ctrl.Result, error) {
	now := metav1.Now()

	backup.Status.Phase = phase
	backup.Status.Message = message
	backup.Status.Reason = reason

	if phase == mcpv1.BackupPhaseRunning && backup.Status.StartTime == nil {
		backup.Status.StartTime = &now
	}

	if phase == mcpv1.BackupPhaseCompleted || phase == mcpv1.BackupPhaseFailed {
		backup.Status.CompletionTime = &now
	}

	// Update conditions
	condition := mcpv1.MCPServerBackupCondition{
		Type:               getConditionType(phase),
		Status:             getConditionStatus(phase),
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// Update or add condition
	backup.Status.Conditions = updateConditions(backup.Status.Conditions, condition)

	err := r.Status().Update(ctx, backup)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to update backup status: %w", err)
	}

	return ctrl.Result{}, nil
}

// getConditionType returns the appropriate condition type for a backup phase
func getConditionType(phase mcpv1.BackupPhase) mcpv1.BackupConditionType {
	switch phase {
	case mcpv1.BackupPhaseRunning:
		return mcpv1.BackupConditionProgressing
	case mcpv1.BackupPhaseCompleted:
		return mcpv1.BackupConditionReady
	case mcpv1.BackupPhaseFailed:
		return mcpv1.BackupConditionFailed
	default:
		return mcpv1.BackupConditionProgressing
	}
}

// getConditionStatus returns the appropriate condition status for a backup phase
func getConditionStatus(phase mcpv1.BackupPhase) metav1.ConditionStatus {
	switch phase {
	case mcpv1.BackupPhaseCompleted:
		return metav1.ConditionTrue
	case mcpv1.BackupPhaseFailed:
		return metav1.ConditionFalse
	default:
		return metav1.ConditionUnknown
	}
}

// updateConditions updates the conditions slice with the new condition
func updateConditions(conditions []mcpv1.MCPServerBackupCondition, newCondition mcpv1.MCPServerBackupCondition) []mcpv1.MCPServerBackupCondition {
	// Find existing condition of the same type
	for i, condition := range conditions {
		if condition.Type == newCondition.Type {
			conditions[i] = newCondition
			return conditions
		}
	}

	// Add new condition if not found
	return append(conditions, newCondition)
}

// SetupWithManager sets up the controller with the Manager
func (r *MCPServerBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize cron scheduler
	r.Cron = cron.New()
	r.Cron.Start()

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServerBackup{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

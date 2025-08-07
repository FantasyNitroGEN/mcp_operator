package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/metrics"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/version"
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
	// Create a structured logger with correlation ID
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
		return ctrl.Result{RequeueAfter: time.Second * 1}, nil
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
	logger.Info("Performing backup cleanup operations", "storage_type", backup.Spec.StorageLocation.Type)

	startTime := time.Now()

	// Perform cleanup based on a storage type
	var err error
	switch backup.Spec.StorageLocation.Type {
	case mcpv1.StorageTypeLocal:
		err = r.cleanupLocalBackup(ctx, logger, backup)
	case mcpv1.StorageTypeS3:
		err = r.cleanupS3Backup(ctx, logger, backup)
	case mcpv1.StorageTypeGCS:
		err = r.cleanupGCSBackup(ctx, logger, backup)
	case mcpv1.StorageTypeAzure:
		err = r.cleanupAzureBackup(ctx, logger, backup)
	default:
		err = fmt.Errorf("unsupported storage type: %s", backup.Spec.StorageLocation.Type)
	}

	// Record cleanup metrics
	duration := time.Since(startTime).Seconds()
	operation := fmt.Sprintf("backup_cleanup_%s", string(backup.Spec.StorageLocation.Type))

	if err != nil {
		logger.Error(err, "Failed to cleanup backup", "storage_type", backup.Spec.StorageLocation.Type)
		metrics.RecordCleanupOperation(backup.Namespace, operation, "error", duration)
		return fmt.Errorf("failed to cleanup backup: %w", err)
	}

	logger.Info("Backup cleanup completed successfully", "storage_type", backup.Spec.StorageLocation.Type, "duration", duration)
	metrics.RecordCleanupOperation(backup.Namespace, operation, "success", duration)

	return nil
}

// cleanupLocalBackup cleans up local backup data
func (r *MCPServerBackupReconciler) cleanupLocalBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up local backup data")

	// Clear backup data from the resource status
	if backup.Status.BackupData != nil {
		logger.Info("Clearing backup data from resource status")
		backup.Status.BackupData = nil
		backup.Status.BackupSize = 0
		backup.Status.BackupLocation = ""

		// Update the resource to persist the cleanup
		if err := r.Status().Update(ctx, backup); err != nil {
			return fmt.Errorf("failed to update backup status after cleanup: %w", err)
		}
	}

	// Production implementation: Clean up actual backup files and indexes
	if err := r.cleanupBackupFiles(ctx, logger, backup); err != nil {
		logger.Error(err, "Failed to cleanup backup files")
		return fmt.Errorf("failed to cleanup backup files: %w", err)
	}

	if err := r.cleanupTemporaryFiles(ctx, logger, backup); err != nil {
		logger.Error(err, "Failed to cleanup temporary files")
		return fmt.Errorf("failed to cleanup temporary files: %w", err)
	}

	if err := r.removeFromLocalStorageIndex(ctx, logger, backup); err != nil {
		logger.Error(err, "Failed to remove from local storage index")
		return fmt.Errorf("failed to remove from local storage index: %w", err)
	}

	return nil
}

// cleanupBackupFiles deletes backup files from persistent volumes
func (r *MCPServerBackupReconciler) cleanupBackupFiles(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up backup files from persistent volumes")

	// Get the storage configuration
	if backup.Spec.StorageLocation.Local == nil {
		logger.Info("No local storage configuration found, skipping file cleanup")
		return nil
	}

	localStorage := backup.Spec.StorageLocation.Local
	if localStorage.Path == "" {
		logger.Info("No storage path configured, skipping file cleanup")
		return nil
	}

	// Construct the backup file path
	backupFileName := fmt.Sprintf("%s-%s.json", backup.Name, backup.UID)
	backupFilePath := filepath.Join(localStorage.Path, backup.Namespace, backupFileName)

	// Check if the backup file exists
	if _, err := os.Stat(backupFilePath); err != nil {
		if os.IsNotExist(err) {
			logger.Info("Backup file does not exist, nothing to clean up", "path", backupFilePath)
			return nil
		}
		return fmt.Errorf("failed to check backup file status: %w", err)
	}

	// Delete the backup file
	logger.Info("Deleting backup file", "path", backupFilePath)
	if err := os.Remove(backupFilePath); err != nil {
		return fmt.Errorf("failed to delete backup file %s: %w", backupFilePath, err)
	}

	// Try to remove the namespace directory if it's empty
	namespaceDirPath := filepath.Join(localStorage.Path, backup.Namespace)
	if entries, err := os.ReadDir(namespaceDirPath); err == nil && len(entries) == 0 {
		logger.Info("Removing empty namespace directory", "path", namespaceDirPath)
		if err := os.Remove(namespaceDirPath); err != nil {
			logger.Info("Failed to remove empty namespace directory (non-critical)", "path", namespaceDirPath, "error", err)
		}
	}

	logger.Info("Backup file cleanup completed successfully")
	return nil
}

// cleanupTemporaryFiles cleans up any temporary files or directories
func (r *MCPServerBackupReconciler) cleanupTemporaryFiles(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up temporary files and directories")

	// Get the storage configuration
	if backup.Spec.StorageLocation.Local == nil {
		logger.Info("No local storage configuration found, skipping temporary file cleanup")
		return nil
	}

	localStorage := backup.Spec.StorageLocation.Local
	if localStorage.Path == "" {
		logger.Info("No storage path configured, skipping temporary file cleanup")
		return nil
	}

	// Define temporary file patterns
	tempPatterns := []string{
		fmt.Sprintf("%s-%s.tmp", backup.Name, backup.UID),
		fmt.Sprintf("%s-%s.partial", backup.Name, backup.UID),
		fmt.Sprintf(".%s-%s.*", backup.Name, backup.UID),
	}

	tempDirPath := filepath.Join(localStorage.Path, backup.Namespace, "tmp")

	// Clean up temporary files in the namespace directory
	namespaceDirPath := filepath.Join(localStorage.Path, backup.Namespace)
	if err := r.cleanupTempFilesInDirectory(logger, namespaceDirPath, tempPatterns); err != nil {
		logger.Error(err, "Failed to cleanup temporary files in namespace directory", "path", namespaceDirPath)
	}

	// Clean up temporary files in the temp subdirectory
	if err := r.cleanupTempFilesInDirectory(logger, tempDirPath, tempPatterns); err != nil {
		logger.Error(err, "Failed to cleanup temporary files in temp directory", "path", tempDirPath)
	}

	// Try to remove the temp directory if it's empty
	if entries, err := os.ReadDir(tempDirPath); err == nil && len(entries) == 0 {
		logger.Info("Removing empty temp directory", "path", tempDirPath)
		if err := os.Remove(tempDirPath); err != nil {
			logger.Info("Failed to remove empty temp directory (non-critical)", "path", tempDirPath, "error", err)
		}
	}

	logger.Info("Temporary file cleanup completed successfully")
	return nil
}

// cleanupTempFilesInDirectory removes temporary files matching patterns in a directory
func (r *MCPServerBackupReconciler) cleanupTempFilesInDirectory(logger logr.Logger, dirPath string, patterns []string) error {
	// Check if directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		logger.Info("Directory does not exist, skipping cleanup", "path", dirPath)
		return nil
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, fileName)
			if err != nil {
				logger.Error(err, "Failed to match pattern", "pattern", pattern, "filename", fileName)
				continue
			}

			if matched {
				filePath := filepath.Join(dirPath, fileName)
				logger.Info("Deleting temporary file", "path", filePath)
				if err := os.Remove(filePath); err != nil {
					logger.Error(err, "Failed to delete temporary file", "path", filePath)
				}
				break
			}
		}
	}

	return nil
}

// removeFromLocalStorageIndex removes backup entries from local storage indexes
func (r *MCPServerBackupReconciler) removeFromLocalStorageIndex(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Removing backup entry from local storage indexes")

	// Get the storage configuration
	if backup.Spec.StorageLocation.Local == nil {
		logger.Info("No local storage configuration found, skipping index cleanup")
		return nil
	}

	localStorage := backup.Spec.StorageLocation.Local
	if localStorage.Path == "" {
		logger.Info("No storage path configured, skipping index cleanup")
		return nil
	}

	// Define index file paths
	indexFilePaths := []string{
		filepath.Join(localStorage.Path, "backup-index.json"),
		filepath.Join(localStorage.Path, backup.Namespace, "namespace-index.json"),
		filepath.Join(localStorage.Path, ".backup-metadata.json"),
	}

	backupKey := fmt.Sprintf("%s/%s", backup.Namespace, backup.Name)

	for _, indexPath := range indexFilePaths {
		if err := r.removeBackupFromIndexFile(logger, indexPath, backupKey, backup); err != nil {
			logger.Error(err, "Failed to remove backup from index file", "index_path", indexPath)
			// Continue with other index files even if one fails
		}
	}

	logger.Info("Local storage index cleanup completed successfully")
	return nil
}

// removeBackupFromIndexFile removes a backup entry from a specific index file
func (r *MCPServerBackupReconciler) removeBackupFromIndexFile(logger logr.Logger, indexPath, backupKey string, backup *mcpv1.MCPServerBackup) error {
	// Check if index file exists
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		logger.Info("Index file does not exist, nothing to clean up", "path", indexPath)
		return nil
	}

	// Read the index file
	indexData, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("failed to read index file %s: %w", indexPath, err)
	}

	// Parse the index as a generic map
	var index map[string]interface{}
	if err := json.Unmarshal(indexData, &index); err != nil {
		logger.Error(err, "Failed to parse index file, skipping", "path", indexPath)
		return nil // Don't fail the cleanup for corrupted index files
	}

	// Remove the backup entry
	modified := false
	if _, exists := index[backupKey]; exists {
		delete(index, backupKey)
		modified = true
		logger.Info("Removed backup entry from index", "key", backupKey, "index_path", indexPath)
	}

	// Also remove entries by UID if present
	uidKey := string(backup.UID)
	if _, exists := index[uidKey]; exists {
		delete(index, uidKey)
		modified = true
		logger.Info("Removed backup UID entry from index", "uid", uidKey, "index_path", indexPath)
	}

	// Write back the modified index if changes were made
	if modified {
		updatedData, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal updated index: %w", err)
		}

		if err := os.WriteFile(indexPath, updatedData, 0644); err != nil {
			return fmt.Errorf("failed to write updated index file %s: %w", indexPath, err)
		}

		logger.Info("Updated index file", "path", indexPath)
	}

	return nil
}

// cleanupS3Backup cleans up S3 backup data
func (r *MCPServerBackupReconciler) cleanupS3Backup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up S3 backup data")

	// Get S3 storage configuration
	if backup.Spec.StorageLocation.S3 == nil {
		logger.Info("No S3 storage configuration found, skipping S3 cleanup")
		return nil
	}

	s3Storage := backup.Spec.StorageLocation.S3

	// Initialize S3 client
	s3Client, err := r.createS3Client(ctx, logger, backup.Namespace, s3Storage)
	if err != nil {
		return fmt.Errorf("failed to create S3 client: %w", err)
	}

	// Delete backup objects from S3 bucket
	if err := r.deleteS3BackupObjects(ctx, logger, s3Client, s3Storage, backup); err != nil {
		logger.Error(err, "Failed to delete backup objects from S3")
		return fmt.Errorf("failed to delete backup objects: %w", err)
	}

	// Clean up any multipart uploads
	if err := r.cleanupS3MultipartUploads(ctx, logger, s3Client, s3Storage, backup); err != nil {
		logger.Error(err, "Failed to cleanup multipart uploads")
		return fmt.Errorf("failed to cleanup multipart uploads: %w", err)
	}

	// Remove backup metadata from S3
	if err := r.removeS3BackupMetadata(ctx, logger, s3Client, s3Storage, backup); err != nil {
		logger.Error(err, "Failed to remove backup metadata from S3")
		return fmt.Errorf("failed to remove backup metadata: %w", err)
	}

	// Update backup indexes
	if err := r.updateS3BackupIndexes(ctx, logger, s3Client, s3Storage, backup); err != nil {
		logger.Error(err, "Failed to update S3 backup indexes")
		return fmt.Errorf("failed to update backup indexes: %w", err)
	}

	logger.Info("S3 backup cleanup completed successfully")
	return nil
}

// createS3Client creates an S3 client with credentials from Kubernetes secrets
func (r *MCPServerBackupReconciler) createS3Client(ctx context.Context, logger logr.Logger, namespace string, s3Storage *mcpv1.S3Storage) (*s3.Client, error) {
	logger.Info("Creating S3 client", "bucket", s3Storage.Bucket, "region", s3Storage.Region)

	// Get AWS credentials from Kubernetes secret
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      s3Storage.CredentialsSecret,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", s3Storage.CredentialsSecret, err)
	}

	// Extract AWS credentials from secret
	accessKeyID, ok := secret.Data["AWS_ACCESS_KEY_ID"]
	if !ok {
		return nil, fmt.Errorf("AWS_ACCESS_KEY_ID not found in secret %s", s3Storage.CredentialsSecret)
	}

	secretAccessKey, ok := secret.Data["AWS_SECRET_ACCESS_KEY"]
	if !ok {
		return nil, fmt.Errorf("AWS_SECRET_ACCESS_KEY not found in secret %s", s3Storage.CredentialsSecret)
	}

	// Create AWS config with credentials
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(s3Storage.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			string(accessKeyID),
			string(secretAccessKey),
			"", // session token (optional)
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Override endpoint if specified (for S3-compatible storage)
	var s3Client *s3.Client
	if s3Storage.Endpoint != "" {
		logger.Info("Using custom S3 endpoint", "endpoint", s3Storage.Endpoint)
		s3Client = s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s3Storage.Endpoint)
			o.UsePathStyle = true // Required for most S3-compatible services
		})
	} else {
		s3Client = s3.NewFromConfig(cfg)
	}

	logger.Info("S3 client created successfully")
	return s3Client, nil
}

// deleteS3BackupObjects deletes backup objects from S3 bucket
func (r *MCPServerBackupReconciler) deleteS3BackupObjects(ctx context.Context, logger logr.Logger, s3Client *s3.Client, s3Storage *mcpv1.S3Storage, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Deleting backup objects from S3 bucket")

	// Construct the backup object key patterns
	backupKeyPatterns := []string{
		fmt.Sprintf("%s/%s-%s.json", s3Storage.Prefix, backup.Name, backup.UID),
		fmt.Sprintf("%s/%s/%s-%s.json", s3Storage.Prefix, backup.Namespace, backup.Name, backup.UID),
		fmt.Sprintf("%s/%s/%s.json", s3Storage.Prefix, backup.Namespace, backup.Name),
	}

	// Remove empty prefix from patterns if no prefix is configured
	if s3Storage.Prefix == "" {
		backupKeyPatterns = []string{
			fmt.Sprintf("%s-%s.json", backup.Name, backup.UID),
			fmt.Sprintf("%s/%s-%s.json", backup.Namespace, backup.Name, backup.UID),
			fmt.Sprintf("%s/%s.json", backup.Namespace, backup.Name),
		}
	}

	var objectsToDelete []s3types.ObjectIdentifier

	// List objects that match our backup patterns
	for _, pattern := range backupKeyPatterns {
		// Use the pattern as a prefix for listing
		var prefix string
		if strings.Contains(pattern, "*") {
			prefix = strings.Split(pattern, "*")[0]
		} else {
			prefix = pattern
		}

		listInput := &s3.ListObjectsV2Input{
			Bucket: aws.String(s3Storage.Bucket),
			Prefix: aws.String(prefix),
		}

		paginator := s3.NewListObjectsV2Paginator(s3Client, listInput)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				logger.Error(err, "Failed to list S3 objects", "prefix", prefix)
				continue
			}

			for _, obj := range page.Contents {
				if obj.Key != nil {
					// Check if the object key matches our backup patterns
					if r.matchesBackupPattern(*obj.Key, backup) {
						objectsToDelete = append(objectsToDelete, s3types.ObjectIdentifier{
							Key: obj.Key,
						})
						logger.Info("Found backup object to delete", "key", *obj.Key)
					}
				}
			}
		}
	}

	// Delete objects in batches (S3 allows up to 1000 objects per delete request)
	if len(objectsToDelete) > 0 {
		batchSize := 1000
		for i := 0; i < len(objectsToDelete); i += batchSize {
			end := i + batchSize
			if end > len(objectsToDelete) {
				end = len(objectsToDelete)
			}

			batch := objectsToDelete[i:end]
			deleteInput := &s3.DeleteObjectsInput{
				Bucket: aws.String(s3Storage.Bucket),
				Delete: &s3types.Delete{
					Objects: batch,
					Quiet:   aws.Bool(false), // Get detailed response
				},
			}

			result, err := s3Client.DeleteObjects(ctx, deleteInput)
			if err != nil {
				return fmt.Errorf("failed to delete S3 objects: %w", err)
			}

			// Log successful deletions
			for _, deleted := range result.Deleted {
				if deleted.Key != nil {
					logger.Info("Successfully deleted S3 object", "key", *deleted.Key)
				}
			}

			// Log any errors
			for _, deleteError := range result.Errors {
				if deleteError.Key != nil && deleteError.Message != nil {
					logger.Error(fmt.Errorf("S3 delete error: %s", *deleteError.Message), "Failed to delete S3 object", "key", *deleteError.Key)
				}
			}
		}
	} else {
		logger.Info("No backup objects found to delete")
	}

	logger.Info("Backup object deletion completed", "objects_deleted", len(objectsToDelete))
	return nil
}

// matchesBackupPattern checks if an S3 object key matches backup patterns for the given backup
func (r *MCPServerBackupReconciler) matchesBackupPattern(key string, backup *mcpv1.MCPServerBackup) bool {
	// Check if the key contains the backup name and UID
	backupIdentifiers := []string{
		fmt.Sprintf("%s-%s", backup.Name, backup.UID),
		backup.Name,
	}

	for _, identifier := range backupIdentifiers {
		if strings.Contains(key, identifier) {
			// Additional check to ensure it's in the correct namespace path
			if strings.Contains(key, backup.Namespace) || !strings.Contains(key, "/") {
				return true
			}
		}
	}

	return false
}

// cleanupS3MultipartUploads cleans up any incomplete multipart uploads
func (r *MCPServerBackupReconciler) cleanupS3MultipartUploads(ctx context.Context, logger logr.Logger, s3Client *s3.Client, s3Storage *mcpv1.S3Storage, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up S3 multipart uploads")

	// List multipart uploads
	listInput := &s3.ListMultipartUploadsInput{
		Bucket: aws.String(s3Storage.Bucket),
	}

	// Add prefix if configured
	if s3Storage.Prefix != "" {
		listInput.Prefix = aws.String(s3Storage.Prefix)
	}

	paginator := s3.NewListMultipartUploadsPaginator(s3Client, listInput)
	var uploadsToAbort []string

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			logger.Error(err, "Failed to list multipart uploads")
			return fmt.Errorf("failed to list multipart uploads: %w", err)
		}

		for _, upload := range page.Uploads {
			if upload.Key != nil && upload.UploadId != nil {
				// Check if this multipart upload is related to our backup
				if r.matchesBackupPattern(*upload.Key, backup) {
					uploadsToAbort = append(uploadsToAbort, *upload.UploadId)
					logger.Info("Found multipart upload to abort", "key", *upload.Key, "upload_id", *upload.UploadId)
				}
			}
		}
	}

	// Abort multipart uploads
	for _, uploadID := range uploadsToAbort {
		// We need to find the key for this upload ID by listing again
		// This is a limitation of the S3 API - we need the key to abort
		paginator := s3.NewListMultipartUploadsPaginator(s3Client, listInput)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				logger.Error(err, "Failed to list multipart uploads for abort")
				continue
			}

			for _, upload := range page.Uploads {
				if upload.UploadId != nil && *upload.UploadId == uploadID && upload.Key != nil {
					abortInput := &s3.AbortMultipartUploadInput{
						Bucket:   aws.String(s3Storage.Bucket),
						Key:      upload.Key,
						UploadId: upload.UploadId,
					}

					_, err := s3Client.AbortMultipartUpload(ctx, abortInput)
					if err != nil {
						logger.Error(err, "Failed to abort multipart upload", "key", *upload.Key, "upload_id", uploadID)
					} else {
						logger.Info("Successfully aborted multipart upload", "key", *upload.Key, "upload_id", uploadID)
					}
					break
				}
			}
		}
	}

	logger.Info("Multipart upload cleanup completed", "uploads_aborted", len(uploadsToAbort))
	return nil
}

// removeS3BackupMetadata removes backup metadata from S3
func (r *MCPServerBackupReconciler) removeS3BackupMetadata(ctx context.Context, logger logr.Logger, s3Client *s3.Client, s3Storage *mcpv1.S3Storage, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Removing backup metadata from S3")

	// Define metadata object key patterns
	metadataKeyPatterns := []string{
		fmt.Sprintf("%s/.metadata/%s-%s.json", s3Storage.Prefix, backup.Name, backup.UID),
		fmt.Sprintf("%s/%s/.metadata/%s-%s.json", s3Storage.Prefix, backup.Namespace, backup.Name, backup.UID),
		fmt.Sprintf("%s/.backup-metadata/%s/%s.json", s3Storage.Prefix, backup.Namespace, backup.Name),
	}

	// Remove empty prefix from patterns if no prefix is configured
	if s3Storage.Prefix == "" {
		metadataKeyPatterns = []string{
			fmt.Sprintf(".metadata/%s-%s.json", backup.Name, backup.UID),
			fmt.Sprintf("%s/.metadata/%s-%s.json", backup.Namespace, backup.Name, backup.UID),
			fmt.Sprintf(".backup-metadata/%s/%s.json", backup.Namespace, backup.Name),
		}
	}

	var metadataObjectsToDelete []s3types.ObjectIdentifier

	// List and identify metadata objects to delete
	for _, pattern := range metadataKeyPatterns {
		// Use the pattern as a prefix for listing
		var prefix string
		if strings.Contains(pattern, "*") {
			prefix = strings.Split(pattern, "*")[0]
		} else {
			// For exact patterns, get the directory part
			if strings.Contains(pattern, "/") {
				parts := strings.Split(pattern, "/")
				prefix = strings.Join(parts[:len(parts)-1], "/") + "/"
			} else {
				prefix = pattern
			}
		}

		listInput := &s3.ListObjectsV2Input{
			Bucket: aws.String(s3Storage.Bucket),
			Prefix: aws.String(prefix),
		}

		paginator := s3.NewListObjectsV2Paginator(s3Client, listInput)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				logger.Error(err, "Failed to list S3 metadata objects", "prefix", prefix)
				continue
			}

			for _, obj := range page.Contents {
				if obj.Key != nil {
					// Check if the object key matches our backup metadata patterns
					if r.matchesBackupMetadataPattern(*obj.Key, backup) {
						metadataObjectsToDelete = append(metadataObjectsToDelete, s3types.ObjectIdentifier{
							Key: obj.Key,
						})
						logger.Info("Found backup metadata object to delete", "key", *obj.Key)
					}
				}
			}
		}
	}

	// Delete metadata objects in batches
	if len(metadataObjectsToDelete) > 0 {
		batchSize := 1000
		for i := 0; i < len(metadataObjectsToDelete); i += batchSize {
			end := i + batchSize
			if end > len(metadataObjectsToDelete) {
				end = len(metadataObjectsToDelete)
			}

			batch := metadataObjectsToDelete[i:end]
			deleteInput := &s3.DeleteObjectsInput{
				Bucket: aws.String(s3Storage.Bucket),
				Delete: &s3types.Delete{
					Objects: batch,
					Quiet:   aws.Bool(false),
				},
			}

			result, err := s3Client.DeleteObjects(ctx, deleteInput)
			if err != nil {
				return fmt.Errorf("failed to delete S3 metadata objects: %w", err)
			}

			// Log successful deletions
			for _, deleted := range result.Deleted {
				if deleted.Key != nil {
					logger.Info("Successfully deleted S3 metadata object", "key", *deleted.Key)
				}
			}

			// Log any errors
			for _, deleteError := range result.Errors {
				if deleteError.Key != nil && deleteError.Message != nil {
					logger.Error(fmt.Errorf("S3 delete error: %s", *deleteError.Message), "Failed to delete S3 metadata object", "key", *deleteError.Key)
				}
			}
		}
	} else {
		logger.Info("No backup metadata objects found to delete")
	}

	logger.Info("Backup metadata removal completed", "metadata_objects_deleted", len(metadataObjectsToDelete))
	return nil
}

// matchesBackupMetadataPattern checks if an S3 object key matches backup metadata patterns
func (r *MCPServerBackupReconciler) matchesBackupMetadataPattern(key string, backup *mcpv1.MCPServerBackup) bool {
	// Check if the key is in a metadata directory and contains backup identifiers
	if !strings.Contains(key, ".metadata") && !strings.Contains(key, ".backup-metadata") {
		return false
	}

	// Check if the key contains the backup name and UID
	backupIdentifiers := []string{
		fmt.Sprintf("%s-%s", backup.Name, backup.UID),
		backup.Name,
	}

	for _, identifier := range backupIdentifiers {
		if strings.Contains(key, identifier) {
			// Additional check to ensure it's in the correct namespace path
			if strings.Contains(key, backup.Namespace) || !strings.Contains(key, "/") {
				return true
			}
		}
	}

	return false
}

// updateS3BackupIndexes updates backup indexes stored in S3
func (r *MCPServerBackupReconciler) updateS3BackupIndexes(ctx context.Context, logger logr.Logger, s3Client *s3.Client, s3Storage *mcpv1.S3Storage, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Updating S3 backup indexes")

	// Define index file key patterns
	indexKeyPatterns := []string{
		fmt.Sprintf("%s/backup-index.json", s3Storage.Prefix),
		fmt.Sprintf("%s/%s/namespace-index.json", s3Storage.Prefix, backup.Namespace),
		fmt.Sprintf("%s/.backup-metadata.json", s3Storage.Prefix),
	}

	// Remove empty prefix from patterns if no prefix is configured
	if s3Storage.Prefix == "" {
		indexKeyPatterns = []string{
			"backup-index.json",
			fmt.Sprintf("%s/namespace-index.json", backup.Namespace),
			".backup-metadata.json",
		}
	}

	backupKey := fmt.Sprintf("%s/%s", backup.Namespace, backup.Name)

	for _, indexKey := range indexKeyPatterns {
		if err := r.updateS3IndexFile(ctx, logger, s3Client, s3Storage.Bucket, indexKey, backupKey, backup); err != nil {
			logger.Error(err, "Failed to update S3 index file", "index_key", indexKey)
			// Continue with other index files even if one fails
		}
	}

	logger.Info("S3 backup indexes update completed successfully")
	return nil
}

// updateS3IndexFile updates a specific index file in S3 by removing backup entries
func (r *MCPServerBackupReconciler) updateS3IndexFile(ctx context.Context, logger logr.Logger, s3Client *s3.Client, bucket, indexKey, backupKey string, backup *mcpv1.MCPServerBackup) error {
	// Check if index file exists
	getInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(indexKey),
	}

	result, err := s3Client.GetObject(ctx, getInput)
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "not found") {
			logger.Info("Index file does not exist, nothing to update", "key", indexKey)
			return nil
		}
		return fmt.Errorf("failed to get S3 index file %s: %w", indexKey, err)
	}
	defer func() {
		if closeErr := result.Body.Close(); closeErr != nil {
			logger.Error(closeErr, "Failed to close S3 object body", "key", indexKey)
		}
	}()

	// Read the index file content
	indexData := make([]byte, 0)
	buffer := make([]byte, 1024)
	for {
		n, err := result.Body.Read(buffer)
		if n > 0 {
			indexData = append(indexData, buffer[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return fmt.Errorf("failed to read S3 index file %s: %w", indexKey, err)
		}
	}

	// Parse the index as a generic map
	var index map[string]interface{}
	if err := json.Unmarshal(indexData, &index); err != nil {
		logger.Error(err, "Failed to parse S3 index file, skipping", "key", indexKey)
		return nil // Don't fail the cleanup for corrupted index files
	}

	// Remove the backup entry
	modified := false
	if _, exists := index[backupKey]; exists {
		delete(index, backupKey)
		modified = true
		logger.Info("Removed backup entry from S3 index", "key", backupKey, "index_key", indexKey)
	}

	// Also remove entries by UID if present
	uidKey := string(backup.UID)
	if _, exists := index[uidKey]; exists {
		delete(index, uidKey)
		modified = true
		logger.Info("Removed backup UID entry from S3 index", "uid", uidKey, "index_key", indexKey)
	}

	// Write back the modified index if changes were made
	if modified {
		updatedData, err := json.MarshalIndent(index, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal updated S3 index: %w", err)
		}

		putInput := &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(indexKey),
			Body:        strings.NewReader(string(updatedData)),
			ContentType: aws.String("application/json"),
		}

		_, err = s3Client.PutObject(ctx, putInput)
		if err != nil {
			return fmt.Errorf("failed to update S3 index file %s: %w", indexKey, err)
		}

		logger.Info("Updated S3 index file", "key", indexKey)
	}

	return nil
}

// cleanupGCSBackup cleans up Google Cloud Storage backup data (placeholder implementation)
func (r *MCPServerBackupReconciler) cleanupGCSBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up GCS backup data")

	// TODO: Implement GCS backup cleanup when GCS storage is implemented
	// This should:
	// - Delete backup objects from GCS bucket
	// - Clean up any incomplete uploads
	// - Remove backup metadata from GCS
	// - Update backup indexes

	logger.Info("GCS backup cleanup completed (placeholder implementation)")
	return nil
}

// cleanupAzureBackup cleans up Azure Blob Storage backup data (placeholder implementation)
func (r *MCPServerBackupReconciler) cleanupAzureBackup(ctx context.Context, logger logr.Logger, backup *mcpv1.MCPServerBackup) error {
	logger.Info("Cleaning up Azure backup data")

	// TODO: Implement Azure backup cleanup when Azure storage is implemented
	// This should:
	// - Delete backup blobs from Azure container
	// - Clean up any incomplete uploads
	// - Remove backup metadata from Azure
	// - Update backup indexes

	logger.Info("Azure backup cleanup completed (placeholder implementation)")
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

	// Get operator version from build info
	operatorVersion := version.GetOperatorVersion()
	logger.V(1).Info("Retrieved operator version", "version", operatorVersion)

	// Get Kubernetes cluster version
	kubernetesVersion := "unknown"
	if config := ctrl.GetConfigOrDie(); config != nil {
		if clusterVersion, err := version.GetClusterVersionSimple(ctx, config); err != nil {
			logger.Error(err, "Failed to get cluster version, using fallback")
			kubernetesVersion = "unknown"
		} else {
			kubernetesVersion = clusterVersion
			logger.V(1).Info("Retrieved cluster version", "version", kubernetesVersion)
		}
	}

	// Create backup data
	backupData := &mcpv1.MCPServerBackupData{
		MCPServerSpec: mcpServer.Spec,
		Metadata: mcpv1.BackupMetadata{
			BackupVersion:     BackupVersionCurrent,
			OperatorVersion:   operatorVersion,
			KubernetesVersion: kubernetesVersion,
			CreatedBy:         "mcp-operator",
			Labels:            backup.Labels,
			Annotations:       backup.Annotations,
		},
		Dependencies: r.collectDependencies(ctx, mcpServer),
	}

	// Store backup data based on a storage type
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
	// In production this should be stored in persistent storage
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
	return fmt.Errorf("azure backup storage not implemented yet")
}

// collectDependencies collects information about related resources
func (r *MCPServerBackupReconciler) collectDependencies(ctx context.Context, mcpServer *mcpv1.MCPServer) []mcpv1.ResourceReference {
	var dependencies []mcpv1.ResourceReference

	// Collect ConfigMaps referenced by ConfigSources
	for _, configSource := range mcpServer.Spec.ConfigSources {
		if configSource.ConfigMap != nil {
			dependencies = append(dependencies, mcpv1.ResourceReference{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       configSource.ConfigMap.Name,
				Namespace:  mcpServer.Namespace,
			})
		}
	}

	// Collect Secrets referenced by SecretRefs
	for _, secretRef := range mcpServer.Spec.SecretRefs {
		dependencies = append(dependencies, mcpv1.ResourceReference{
			APIVersion: "v1",
			Kind:       "Secret",
			Name:       secretRef.Name,
			Namespace:  mcpServer.Namespace,
		})
	}

	// Collect ConfigMaps and Secrets referenced by EnvFrom
	for _, envFrom := range mcpServer.Spec.EnvFrom {
		if envFrom.ConfigMapRef != nil {
			dependencies = append(dependencies, mcpv1.ResourceReference{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       envFrom.ConfigMapRef.Name,
				Namespace:  mcpServer.Namespace,
			})
		}
		if envFrom.SecretRef != nil {
			dependencies = append(dependencies, mcpv1.ResourceReference{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       envFrom.SecretRef.Name,
				Namespace:  mcpServer.Namespace,
			})
		}
	}

	// Collect resources referenced by Volumes
	for _, volume := range mcpServer.Spec.Volumes {
		if volume.MCPVolumeSource.ConfigMap != nil {
			dependencies = append(dependencies, mcpv1.ResourceReference{
				APIVersion: "v1",
				Kind:       "ConfigMap",
				Name:       volume.MCPVolumeSource.ConfigMap.Name,
				Namespace:  mcpServer.Namespace,
			})
		}
		if volume.MCPVolumeSource.Secret != nil {
			dependencies = append(dependencies, mcpv1.ResourceReference{
				APIVersion: "v1",
				Kind:       "Secret",
				Name:       volume.MCPVolumeSource.Secret.Name,
				Namespace:  mcpServer.Namespace,
			})
		}
		if volume.MCPVolumeSource.PersistentVolumeClaim != nil {
			dependencies = append(dependencies, mcpv1.ResourceReference{
				APIVersion: "v1",
				Kind:       "PersistentVolumeClaim",
				Name:       volume.MCPVolumeSource.PersistentVolumeClaim.ClaimName,
				Namespace:  mcpServer.Namespace,
			})
		}
		if volume.MCPVolumeSource.Projected != nil {
			for _, projection := range volume.MCPVolumeSource.Projected.Sources {
				if projection.ConfigMap != nil {
					dependencies = append(dependencies, mcpv1.ResourceReference{
						APIVersion: "v1",
						Kind:       "ConfigMap",
						Name:       projection.ConfigMap.Name,
						Namespace:  mcpServer.Namespace,
					})
				}
				if projection.Secret != nil {
					dependencies = append(dependencies, mcpv1.ResourceReference{
						APIVersion: "v1",
						Kind:       "Secret",
						Name:       projection.Secret.Name,
						Namespace:  mcpServer.Namespace,
					})
				}
			}
		}
	}

	// Collect Service created for the MCPServer (same name as MCPServer)
	dependencies = append(dependencies, mcpv1.ResourceReference{
		APIVersion: "v1",
		Kind:       "Service",
		Name:       mcpServer.Name,
		Namespace:  mcpServer.Namespace,
	})

	// Collect Deployment created for the MCPServer (same name as MCPServer)
	dependencies = append(dependencies, mcpv1.ResourceReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       mcpServer.Name,
		Namespace:  mcpServer.Namespace,
	})

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

	// Filter out backups that are not completed (don't delete running/failed backups)
	completedBackups := make([]mcpv1.MCPServerBackup, 0)
	for _, b := range backupList.Items {
		if b.Status.Phase == mcpv1.BackupPhaseCompleted {
			completedBackups = append(completedBackups, b)
		}
	}

	if len(completedBackups) == 0 {
		logger.Info("No completed backups found, skipping retention policy")
		return nil
	}

	// Sort backups by creation time (newest first)
	sort.Slice(completedBackups, func(i, j int) bool {
		return completedBackups[i].CreationTimestamp.After(completedBackups[j].CreationTimestamp.Time)
	})

	backupsToDelete := make([]mcpv1.MCPServerBackup, 0)
	policy := backup.Spec.RetentionPolicy

	// Apply KeepLast policy
	if policy.KeepLast != nil && *policy.KeepLast > 0 {
		keepCount := int(*policy.KeepLast)
		if len(completedBackups) > keepCount {
			backupsToDelete = append(backupsToDelete, completedBackups[keepCount:]...)
		}
	}

	// Apply MaxAge policy
	if policy.MaxAge != nil {
		maxAge := policy.MaxAge.Duration
		cutoffTime := time.Now().Add(-maxAge)
		for _, b := range completedBackups {
			if b.CreationTimestamp.Before(&metav1.Time{Time: cutoffTime}) {
				// Check if this backup is not already in the delete list
				found := false
				for _, toDelete := range backupsToDelete {
					if toDelete.Name == b.Name && toDelete.Namespace == b.Namespace {
						found = true
						break
					}
				}
				if !found {
					backupsToDelete = append(backupsToDelete, b)
				}
			}
		}
	}

	// Delete the backups that exceed retention policy
	deletedCount := 0
	for _, backupToDelete := range backupsToDelete {
		logger.Info("Deleting backup due to retention policy",
			"backup", backupToDelete.Name,
			"creationTime", backupToDelete.CreationTimestamp.Time)

		err := r.Delete(ctx, &backupToDelete)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("Backup already deleted", "backup", backupToDelete.Name)
				continue
			}
			logger.Error(err, "Failed to delete backup", "backup", backupToDelete.Name)
			return fmt.Errorf("failed to delete backup %s: %w", backupToDelete.Name, err)
		}
		deletedCount++
	}

	logger.Info("Retention policy enforced",
		"total_backups", len(backupList.Items),
		"completed_backups", len(completedBackups),
		"deleted_backups", deletedCount)
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

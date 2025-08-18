/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package services

import (
	"context"
	"fmt"
	"time"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultAutoUpdateService implements AutoUpdateService
type DefaultAutoUpdateService struct {
	client          client.Client
	registryService RegistryService
	statusService   StatusService
	eventService    EventService
	stopChan        chan struct{}
	isRunning       bool
	failureTracker  map[string]*UpdateFailureInfo
}

// UpdateFailureInfo tracks failure information for backoff logic
type UpdateFailureInfo struct {
	FailureCount    int
	LastFailureTime time.Time
	NextRetryTime   time.Time
}

// NewDefaultAutoUpdateService creates a new DefaultAutoUpdateService
func NewDefaultAutoUpdateService(
	client client.Client,
	registryService RegistryService,
	statusService StatusService,
	eventService EventService,
) *DefaultAutoUpdateService {
	return &DefaultAutoUpdateService{
		client:          client,
		registryService: registryService,
		statusService:   statusService,
		eventService:    eventService,
		stopChan:        make(chan struct{}),
		isRunning:       false,
		failureTracker:  make(map[string]*UpdateFailureInfo),
	}
}

// CheckForUpdates checks if any MCPServers need updates from registry
func (s *DefaultAutoUpdateService) CheckForUpdates(ctx context.Context) error {
	logger := log.FromContext(ctx).WithValues("component", "auto-update-service")
	logger.Info("Starting periodic check for template updates")

	// List all MCPServers
	mcpServerList := &mcpv1.MCPServerList{}
	if err := s.client.List(ctx, mcpServerList); err != nil {
		logger.Error(err, "Failed to list MCPServers")
		return fmt.Errorf("failed to list MCPServers: %w", err)
	}

	updatedCount := 0
	errorCount := 0

	// Check each MCPServer for updates
	for _, mcpServer := range mcpServerList.Items {
		// Skip servers without registry configuration
		if mcpServer.Spec.Registry == nil || (mcpServer.Spec.Registry.Registry == "" && mcpServer.Spec.Registry.Name == "") {
			continue
		}

		// Get registry name for logging (prefer new field, fall back to old)
		registryName := mcpServer.Spec.Registry.Name
		if registryName == "" && mcpServer.Spec.Registry.Registry != "" {
			registryName = mcpServer.Spec.Registry.Registry
		}

		serverKey := fmt.Sprintf("%s/%s", mcpServer.Namespace, mcpServer.Name)
		serverLogger := logger.WithValues(
			"mcpserver", mcpServer.Name,
			"namespace", mcpServer.Namespace,
			"registry", registryName,
		)

		// Check if server is in backoff period
		if s.isInBackoffPeriod(serverKey) {
			failureInfo := s.failureTracker[serverKey]
			serverLogger.V(1).Info("Skipping server in backoff period",
				"failure_count", failureInfo.FailureCount,
				"next_retry_time", failureInfo.NextRetryTime,
			)
			continue
		}

		// Check if update is required
		updateRequired, latestSpec, err := s.IsUpdateRequired(ctx, &mcpServer)
		if err != nil {
			serverLogger.Error(err, "Failed to check if update is required")
			errorCount++
			continue
		}

		if !updateRequired {
			serverLogger.V(1).Info("No update required for MCPServer")
			continue
		}

		serverLogger.Info("Update required for MCPServer",
			"current_image", mcpServer.Spec.Runtime.Image,
			"latest_image", latestSpec.Runtime.Image,
		)

		// Update the server
		updated, err := s.UpdateServerFromTemplate(ctx, &mcpServer)
		if err != nil {
			serverLogger.Error(err, "Failed to update MCPServer from template")

			// Record failure for backoff calculation
			s.recordFailure(serverKey)

			// Set error condition on MCPServer
			s.statusService.SetCondition(&mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "TemplateUpdateFailed",
				fmt.Sprintf("Failed to update from template: %v", err))

			// Record warning event
			s.eventService.RecordWarning(&mcpServer, "TemplateUpdateFailed",
				fmt.Sprintf("Failed to update from template: %v", err))

			// Update status to reflect the error
			if statusErr := s.statusService.UpdateMCPServerStatus(ctx, &mcpServer); statusErr != nil {
				serverLogger.Error(statusErr, "Failed to update MCPServer status after template update failure")
			}

			errorCount++
			continue
		}

		if updated {
			// Record success and clear any failure tracking
			s.recordSuccess(serverKey)
			updatedCount++
			serverLogger.Info("Successfully updated MCPServer from template")
		}
	}

	logger.Info("Completed periodic check for template updates",
		"total_servers", len(mcpServerList.Items),
		"updated_servers", updatedCount,
		"errors", errorCount,
	)

	return nil
}

// UpdateServerFromTemplate updates a specific MCPServer with latest template data
func (s *DefaultAutoUpdateService) UpdateServerFromTemplate(ctx context.Context, mcpServer *mcpv1.MCPServer) (bool, error) {
	// Get registry name for logging (prefer new field, fall back to old)
	registryName := mcpServer.Spec.Registry.Name
	if registryName == "" && mcpServer.Spec.Registry.Registry != "" {
		registryName = mcpServer.Spec.Registry.Registry
	}

	logger := log.FromContext(ctx).WithValues(
		"mcpserver", mcpServer.Name,
		"namespace", mcpServer.Namespace,
		"registry", registryName,
	)

	// Check if update is required
	updateRequired, latestSpec, err := s.IsUpdateRequired(ctx, mcpServer)
	if err != nil {
		return false, fmt.Errorf("failed to check if update is required: %w", err)
	}

	if !updateRequired {
		logger.V(1).Info("No update required for MCPServer")
		return false, nil
	}

	// Store original values for comparison
	originalVersion := "" // RegistryRef doesn't have Version field
	originalImage := mcpServer.Spec.Runtime.Image

	// Update MCPServer with latest template data
	s.applyTemplateUpdates(mcpServer, latestSpec)

	// Update the MCPServer resource
	if err := s.client.Update(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to update MCPServer resource")

		// Set error condition
		s.statusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionFalse), "TemplateUpdateResourceFailed",
			fmt.Sprintf("Failed to update MCPServer resource: %v", err))

		// Record warning event
		s.eventService.RecordWarning(mcpServer, "TemplateUpdateResourceFailed",
			fmt.Sprintf("Failed to update MCPServer resource: %v", err))

		// Try to update status even if resource update failed
		if statusErr := s.statusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status after resource update failure")
		}

		return false, fmt.Errorf("failed to update MCPServer resource: %w", err)
	}

	// Set condition to indicate template update
	s.statusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRegistryFetched,
		string(metav1.ConditionTrue), "TemplateUpdated",
		fmt.Sprintf("Updated from template version %s to %s", originalVersion, latestSpec.Version))

	// Record event
	s.eventService.RecordNormal(mcpServer, "TemplateUpdated",
		fmt.Sprintf("Updated from template version %s to %s, image %s to %s",
			originalVersion, latestSpec.Version, originalImage, latestSpec.Runtime.Image))

	logger.Info("Successfully updated MCPServer from template",
		"old_version", originalVersion,
		"new_version", latestSpec.Version,
		"old_image", originalImage,
		"new_image", latestSpec.Runtime.Image,
	)

	return true, nil
}

// IsUpdateRequired checks if an MCPServer needs updating based on registry template
func (s *DefaultAutoUpdateService) IsUpdateRequired(ctx context.Context, mcpServer *mcpv1.MCPServer) (bool, *registry.MCPServerSpec, error) {
	// Get registry name for logging (prefer new field, fall back to old)
	registryName := mcpServer.Spec.Registry.Name
	if registryName == "" && mcpServer.Spec.Registry.Registry != "" {
		registryName = mcpServer.Spec.Registry.Registry
	}

	logger := log.FromContext(ctx).WithValues(
		"mcpserver", mcpServer.Name,
		"namespace", mcpServer.Namespace,
		"registry", registryName,
	)

	// Skip if no registry is configured - use current fields only
	if mcpServer.Spec.Registry.Name == "" && mcpServer.Spec.Registry.Registry == "" {
		return false, nil, nil
	}

	// Fetch latest server specification from registry - registryName already set above
	// if registryName is still empty, fall back to Registry field
	if registryName == "" {
		registryName = mcpServer.Spec.Registry.Registry
	}
	serverName := mcpServer.Spec.Registry.ServerName
	if serverName == "" {
		serverName = mcpServer.Name // default to MCPServer name
	}
	latestSpec, err := s.registryService.FetchServerSpec(ctx, registryName, serverName)
	if err != nil {
		logger.Error(err, "Failed to fetch latest server specification")
		return false, nil, fmt.Errorf("failed to fetch latest server specification: %w", err)
	}

	// Compare template digests first - this catches any template changes
	currentDigest := ""
	if mcpServer.Annotations != nil {
		currentDigest = mcpServer.Annotations["mcp.allbeone.io/template-digest"]
	}

	if currentDigest != "" && latestSpec.TemplateDigest != "" && currentDigest != latestSpec.TemplateDigest {
		logger.Info("Template digest mismatch detected",
			"current_digest", currentDigest,
			"latest_digest", latestSpec.TemplateDigest,
		)
		return true, latestSpec, nil
	}

	// Compare versions using annotations since RegistryRef doesn't have Version field
	currentVersion := ""
	if mcpServer.Annotations != nil {
		currentVersion = mcpServer.Annotations["mcp.allbeone.io/registry-version"]
	}
	if currentVersion != latestSpec.Version {
		logger.Info("Version mismatch detected",
			"current_version", currentVersion,
			"latest_version", latestSpec.Version,
		)
		return true, latestSpec, nil
	}

	// Compare images
	if mcpServer.Spec.Runtime.Image != latestSpec.Runtime.Image {
		logger.Info("Image mismatch detected",
			"current_image", mcpServer.Spec.Runtime.Image,
			"latest_image", latestSpec.Runtime.Image,
		)
		return true, latestSpec, nil
	}

	// Compare other critical fields that might affect deployment
	if !s.areRuntimeSpecsEqual(mcpServer.Spec.Runtime, latestSpec.Runtime) {
		logger.Info("Runtime specification mismatch detected")
		return true, latestSpec, nil
	}

	return false, latestSpec, nil
}

// StartPeriodicSync starts periodic synchronization of templates
func (s *DefaultAutoUpdateService) StartPeriodicSync(ctx context.Context, interval time.Duration) error {
	if s.isRunning {
		return fmt.Errorf("periodic sync is already running")
	}

	logger := log.FromContext(ctx).WithValues("component", "auto-update-service")
	logger.Info("Starting periodic template synchronization", "interval", interval)

	s.isRunning = true
	s.stopChan = make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.CheckForUpdates(ctx); err != nil {
					logger.Error(err, "Error during periodic template check")
				}
			case <-s.stopChan:
				logger.Info("Stopping periodic template synchronization")
				return
			}
		}
	}()

	return nil
}

// StopPeriodicSync stops periodic synchronization
func (s *DefaultAutoUpdateService) StopPeriodicSync() {
	if s.isRunning {
		close(s.stopChan)
		s.isRunning = false
	}
}

// applyTemplateUpdates applies template updates to MCPServer
func (s *DefaultAutoUpdateService) applyTemplateUpdates(mcpServer *mcpv1.MCPServer, latestSpec *registry.MCPServerSpec) {
	// Update registry metadata in annotations (since RegistryRef only has Registry and Server fields)
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}
	mcpServer.Annotations["mcp.allbeone.io/registry-version"] = latestSpec.Version
	mcpServer.Annotations["mcp.allbeone.io/registry-description"] = latestSpec.Description
	mcpServer.Annotations["mcp.allbeone.io/registry-repository"] = latestSpec.Repository
	mcpServer.Annotations["mcp.allbeone.io/registry-license"] = latestSpec.License
	mcpServer.Annotations["mcp.allbeone.io/registry-author"] = latestSpec.Author

	// Update runtime information
	mcpServer.Spec.Runtime.Type = latestSpec.Runtime.Type
	mcpServer.Spec.Runtime.Image = latestSpec.Runtime.Image
	mcpServer.Spec.Runtime.Command = latestSpec.Runtime.Command
	mcpServer.Spec.Runtime.Args = latestSpec.Runtime.Args

	// Update environment variables (merge with existing)
	if mcpServer.Spec.Runtime.Env == nil {
		mcpServer.Spec.Runtime.Env = make(map[string]string)
	}
	for k, v := range latestSpec.Runtime.Env {
		mcpServer.Spec.Runtime.Env[k] = v
	}

	// Update template digest annotation
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}
	if latestSpec.TemplateDigest != "" {
		mcpServer.Annotations["mcp.allbeone.io/template-digest"] = latestSpec.TemplateDigest
	}
}

// areRuntimeSpecsEqual compares mcpv1.RuntimeSpec with registry.RuntimeSpec
func (s *DefaultAutoUpdateService) areRuntimeSpecsEqual(current *mcpv1.RuntimeSpec, latest registry.RuntimeSpec) bool {
	// Compare type
	if current.Type != latest.Type {
		return false
	}

	// Compare command
	if len(current.Command) != len(latest.Command) {
		return false
	}
	for i, cmd := range current.Command {
		if cmd != latest.Command[i] {
			return false
		}
	}

	// Compare args
	if len(current.Args) != len(latest.Args) {
		return false
	}
	for i, arg := range current.Args {
		if arg != latest.Args[i] {
			return false
		}
	}

	return true
}

// isInBackoffPeriod checks if a server is currently in backoff period
func (s *DefaultAutoUpdateService) isInBackoffPeriod(serverKey string) bool {
	failureInfo, exists := s.failureTracker[serverKey]
	if !exists {
		return false
	}

	return time.Now().Before(failureInfo.NextRetryTime)
}

// recordFailure records a failure for backoff calculation
func (s *DefaultAutoUpdateService) recordFailure(serverKey string) {
	now := time.Now()

	if failureInfo, exists := s.failureTracker[serverKey]; exists {
		failureInfo.FailureCount++
		failureInfo.LastFailureTime = now
		failureInfo.NextRetryTime = now.Add(s.calculateBackoffDuration(failureInfo.FailureCount))
	} else {
		s.failureTracker[serverKey] = &UpdateFailureInfo{
			FailureCount:    1,
			LastFailureTime: now,
			NextRetryTime:   now.Add(s.calculateBackoffDuration(1)),
		}
	}
}

// recordSuccess records a successful update and clears failure tracking
func (s *DefaultAutoUpdateService) recordSuccess(serverKey string) {
	delete(s.failureTracker, serverKey)
}

// calculateBackoffDuration calculates exponential backoff duration
func (s *DefaultAutoUpdateService) calculateBackoffDuration(failureCount int) time.Duration {
	// Exponential backoff: 2^failureCount minutes, capped at 24 hours
	backoffMinutes := 1 << uint(failureCount-1) // 2^(failureCount-1)
	if backoffMinutes > 1440 {                  // Cap at 24 hours (1440 minutes)
		backoffMinutes = 1440
	}

	return time.Duration(backoffMinutes) * time.Minute
}

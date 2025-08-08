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

package controllers

import (
	"context"
	"time"

	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// DefaultAutoUpdateInterval is the default interval for auto-updating templates
	DefaultAutoUpdateInterval = 24 * time.Hour // Check for updates daily

	// AutoUpdateControllerName is the name of the auto-update controller
	AutoUpdateControllerName = "auto-update-controller"
)

// AutoUpdateController manages periodic template updates
type AutoUpdateController struct {
	autoUpdateService services.AutoUpdateService
	interval          time.Duration
	stopChan          chan struct{}
	isRunning         bool
}

// NewAutoUpdateController creates a new AutoUpdateController
func NewAutoUpdateController(autoUpdateService services.AutoUpdateService, interval time.Duration) *AutoUpdateController {
	if interval <= 0 {
		interval = DefaultAutoUpdateInterval
	}

	return &AutoUpdateController{
		autoUpdateService: autoUpdateService,
		interval:          interval,
		stopChan:          make(chan struct{}),
		isRunning:         false,
	}
}

// Start starts the auto-update controller
func (c *AutoUpdateController) Start(ctx context.Context) error {
	if c.isRunning {
		return nil
	}

	logger := log.FromContext(ctx).WithValues("controller", AutoUpdateControllerName)
	logger.Info("Starting auto-update controller", "interval", c.interval)

	c.isRunning = true
	c.stopChan = make(chan struct{})

	// Start the periodic sync in the auto-update service
	if err := c.autoUpdateService.StartPeriodicSync(ctx, c.interval); err != nil {
		logger.Error(err, "Failed to start periodic sync in auto-update service")
		c.isRunning = false
		return err
	}

	// Start the controller's own monitoring goroutine
	go c.run(ctx)

	logger.Info("Auto-update controller started successfully")
	return nil
}

// Stop stops the auto-update controller
func (c *AutoUpdateController) Stop() {
	if !c.isRunning {
		return
	}

	logger := log.FromContext(context.Background()).WithValues("controller", AutoUpdateControllerName)
	logger.Info("Stopping auto-update controller")

	// Stop the periodic sync in the auto-update service
	c.autoUpdateService.StopPeriodicSync()

	// Stop the controller's monitoring goroutine
	close(c.stopChan)
	c.isRunning = false

	logger.Info("Auto-update controller stopped")
}

// run is the main loop of the auto-update controller
func (c *AutoUpdateController) run(ctx context.Context) {
	logger := log.FromContext(ctx).WithValues("controller", AutoUpdateControllerName)

	// Create a ticker for health checks and monitoring
	healthCheckInterval := 1 * time.Hour
	ticker := time.NewTicker(healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Perform health check or monitoring tasks
			logger.V(1).Info("Auto-update controller health check", "status", "running")

		case <-c.stopChan:
			logger.Info("Auto-update controller run loop stopped")
			return

		case <-ctx.Done():
			logger.Info("Auto-update controller context cancelled")
			return
		}
	}
}

// NeedLeaderElection returns true if the controller needs leader election
func (c *AutoUpdateController) NeedLeaderElection() bool {
	return true
}

// Runnable interface implementation for controller-runtime manager
type AutoUpdateRunnable struct {
	controller *AutoUpdateController
}

// NewAutoUpdateRunnable creates a new AutoUpdateRunnable
func NewAutoUpdateRunnable(autoUpdateService services.AutoUpdateService, interval time.Duration) *AutoUpdateRunnable {
	return &AutoUpdateRunnable{
		controller: NewAutoUpdateController(autoUpdateService, interval),
	}
}

// Start implements the manager.Runnable interface
func (r *AutoUpdateRunnable) Start(ctx context.Context) error {
	return r.controller.Start(ctx)
}

// NeedLeaderElection implements the manager.LeaderElectionRunnable interface
func (r *AutoUpdateRunnable) NeedLeaderElection() bool {
	return r.controller.NeedLeaderElection()
}

// AddAutoUpdateController adds the auto-update controller to the manager
func AddAutoUpdateController(mgr manager.Manager, autoUpdateService services.AutoUpdateService, interval time.Duration) error {
	logger := mgr.GetLogger().WithValues("component", "auto-update-setup")
	logger.Info("Adding auto-update controller to manager", "interval", interval)

	runnable := NewAutoUpdateRunnable(autoUpdateService, interval)

	if err := mgr.Add(runnable); err != nil {
		logger.Error(err, "Failed to add auto-update controller to manager")
		return err
	}

	logger.Info("Auto-update controller added to manager successfully")
	return nil
}

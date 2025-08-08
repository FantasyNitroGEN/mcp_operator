package services

import (
	"context"
	"sync"
	"time"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// CacheEntry represents a cached item with expiration
type CacheEntry struct {
	Data      interface{}
	ExpiresAt time.Time
}

// IsExpired checks if the cache entry has expired
func (c *CacheEntry) IsExpired() bool {
	return time.Now().After(c.ExpiresAt)
}

// CacheService provides caching functionality for frequently accessed objects
type CacheService interface {
	// Registry caching
	GetRegistry(ctx context.Context, key string) (*mcpv1.MCPRegistry, bool)
	SetRegistry(ctx context.Context, key string, registry *mcpv1.MCPRegistry, ttl time.Duration)
	InvalidateRegistry(ctx context.Context, key string)

	// MCPServer caching
	GetMCPServer(ctx context.Context, key string) (*mcpv1.MCPServer, bool)
	SetMCPServer(ctx context.Context, key string, server *mcpv1.MCPServer, ttl time.Duration)
	InvalidateMCPServer(ctx context.Context, key string)

	// Registry server list caching
	GetRegistryServers(ctx context.Context, registryName string) ([]mcpv1.MCPServer, bool)
	SetRegistryServers(ctx context.Context, registryName string, servers []mcpv1.MCPServer, ttl time.Duration)
	InvalidateRegistryServers(ctx context.Context, registryName string)

	// Cache management
	ClearExpired(ctx context.Context)
	ClearAll(ctx context.Context)
	GetStats(ctx context.Context) CacheStats
}

// CacheStats provides statistics about cache usage
type CacheStats struct {
	RegistryHits          int64
	RegistryMisses        int64
	MCPServerHits         int64
	MCPServerMisses       int64
	RegistryServersHits   int64
	RegistryServersMisses int64
	TotalEntries          int
	ExpiredEntries        int
}

// DefaultCacheService implements CacheService with in-memory caching
type DefaultCacheService struct {
	registryCache        map[string]*CacheEntry
	mcpServerCache       map[string]*CacheEntry
	registryServersCache map[string]*CacheEntry
	mutex                sync.RWMutex
	stats                CacheStats
	logger               logr.Logger
}

// NewDefaultCacheService creates a new cache service
func NewDefaultCacheService() *DefaultCacheService {
	return &DefaultCacheService{
		registryCache:        make(map[string]*CacheEntry),
		mcpServerCache:       make(map[string]*CacheEntry),
		registryServersCache: make(map[string]*CacheEntry),
		logger:               log.Log.WithName("cache-service"),
	}
}

// GetRegistry retrieves a registry from a cache
func (c *DefaultCacheService) GetRegistry(ctx context.Context, key string) (*mcpv1.MCPRegistry, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.registryCache[key]
	if !exists || entry.IsExpired() {
		c.stats.RegistryMisses++
		if exists && entry.IsExpired() {
			// Clean up expired entry
			go func() {
				c.mutex.Lock()
				delete(c.registryCache, key)
				c.mutex.Unlock()
			}()
		}
		return nil, false
	}

	c.stats.RegistryHits++
	registry, ok := entry.Data.(*mcpv1.MCPRegistry)
	return registry, ok
}

// SetRegistry stores a registry in cache
func (c *DefaultCacheService) SetRegistry(ctx context.Context, key string, registry *mcpv1.MCPRegistry, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Create a deep copy to avoid reference issues
	registryCopy := registry.DeepCopy()

	c.registryCache[key] = &CacheEntry{
		Data:      registryCopy,
		ExpiresAt: time.Now().Add(ttl),
	}

	c.logger.V(1).Info("Registry cached", "key", key, "ttl", ttl)
}

// InvalidateRegistry removes a registry from cache
func (c *DefaultCacheService) InvalidateRegistry(ctx context.Context, key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.registryCache, key)
	c.logger.V(1).Info("Registry cache invalidated", "key", key)
}

// GetMCPServer retrieves an MCPServer from cache
func (c *DefaultCacheService) GetMCPServer(ctx context.Context, key string) (*mcpv1.MCPServer, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.mcpServerCache[key]
	if !exists || entry.IsExpired() {
		c.stats.MCPServerMisses++
		if exists && entry.IsExpired() {
			// Clean up expired entry
			go func() {
				c.mutex.Lock()
				delete(c.mcpServerCache, key)
				c.mutex.Unlock()
			}()
		}
		return nil, false
	}

	c.stats.MCPServerHits++
	server, ok := entry.Data.(*mcpv1.MCPServer)
	return server, ok
}

// SetMCPServer stores an MCPServer in cache
func (c *DefaultCacheService) SetMCPServer(ctx context.Context, key string, server *mcpv1.MCPServer, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Create a deep copy to avoid reference issues
	serverCopy := server.DeepCopy()

	c.mcpServerCache[key] = &CacheEntry{
		Data:      serverCopy,
		ExpiresAt: time.Now().Add(ttl),
	}

	c.logger.V(1).Info("MCPServer cached", "key", key, "ttl", ttl)
}

// InvalidateMCPServer removes an MCPServer from cache
func (c *DefaultCacheService) InvalidateMCPServer(ctx context.Context, key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.mcpServerCache, key)
	c.logger.V(1).Info("MCPServer cache invalidated", "key", key)
}

// GetRegistryServers retrieves registry servers list from cache
func (c *DefaultCacheService) GetRegistryServers(ctx context.Context, registryName string) ([]mcpv1.MCPServer, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	entry, exists := c.registryServersCache[registryName]
	if !exists || entry.IsExpired() {
		c.stats.RegistryServersMisses++
		if exists && entry.IsExpired() {
			// Clean up expired entry
			go func() {
				c.mutex.Lock()
				delete(c.registryServersCache, registryName)
				c.mutex.Unlock()
			}()
		}
		return nil, false
	}

	c.stats.RegistryServersHits++
	servers, ok := entry.Data.([]mcpv1.MCPServer)
	return servers, ok
}

// SetRegistryServers stores registry servers list in cache
func (c *DefaultCacheService) SetRegistryServers(ctx context.Context, registryName string, servers []mcpv1.MCPServer, ttl time.Duration) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Create deep copies to avoid reference issues
	serversCopy := make([]mcpv1.MCPServer, len(servers))
	for i, server := range servers {
		serversCopy[i] = *server.DeepCopy()
	}

	c.registryServersCache[registryName] = &CacheEntry{
		Data:      serversCopy,
		ExpiresAt: time.Now().Add(ttl),
	}

	c.logger.V(1).Info("Registry servers cached", "registryName", registryName, "count", len(servers), "ttl", ttl)
}

// InvalidateRegistryServers removes registry servers list from cache
func (c *DefaultCacheService) InvalidateRegistryServers(ctx context.Context, registryName string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.registryServersCache, registryName)
	c.logger.V(1).Info("Registry servers cache invalidated", "registryName", registryName)
}

// ClearExpired removes all expired entries from cache
func (c *DefaultCacheService) ClearExpired(ctx context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	expiredCount := 0

	// Clear expired registry entries
	for key, entry := range c.registryCache {
		if entry.IsExpired() {
			delete(c.registryCache, key)
			expiredCount++
		}
	}

	// Clear expired MCPServer entries
	for key, entry := range c.mcpServerCache {
		if entry.IsExpired() {
			delete(c.mcpServerCache, key)
			expiredCount++
		}
	}

	// Clear expired registry servers entries
	for key, entry := range c.registryServersCache {
		if entry.IsExpired() {
			delete(c.registryServersCache, key)
			expiredCount++
		}
	}

	if expiredCount > 0 {
		c.logger.Info("Cleared expired cache entries", "count", expiredCount)
	}
}

// ClearAll removes all entries from cache
func (c *DefaultCacheService) ClearAll(ctx context.Context) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	totalEntries := len(c.registryCache) + len(c.mcpServerCache) + len(c.registryServersCache)

	c.registryCache = make(map[string]*CacheEntry)
	c.mcpServerCache = make(map[string]*CacheEntry)
	c.registryServersCache = make(map[string]*CacheEntry)

	c.logger.Info("Cleared all cache entries", "count", totalEntries)
}

// GetStats returns cache statistics
func (c *DefaultCacheService) GetStats(ctx context.Context) CacheStats {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	stats := c.stats
	stats.TotalEntries = len(c.registryCache) + len(c.mcpServerCache) + len(c.registryServersCache)

	// Count expired entries
	expiredCount := 0
	for _, entry := range c.registryCache {
		if entry.IsExpired() {
			expiredCount++
		}
	}
	for _, entry := range c.mcpServerCache {
		if entry.IsExpired() {
			expiredCount++
		}
	}
	for _, entry := range c.registryServersCache {
		if entry.IsExpired() {
			expiredCount++
		}
	}
	stats.ExpiredEntries = expiredCount

	return stats
}

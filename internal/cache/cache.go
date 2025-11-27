package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache defines the interface for caching rendered content
type Cache interface {
	// Get retrieves a cached value by key, returning the value and whether it was found
	Get(key string) (string, bool)
	// Set stores a value in the cache with the given key
	Set(key string, value string) error
	// Clear removes all entries from the cache
	Clear() error
}

// LRUMemoryCache implements a simple in-memory LRU cache
type LRUMemoryCache struct {
	maxSize int
	mu      sync.RWMutex
	items   map[string]string
	order   []string // Track order for LRU eviction
}

// NewLRUMemoryCache creates a new in-memory LRU cache with a maximum size
func NewLRUMemoryCache(maxSize int) *LRUMemoryCache {
	if maxSize <= 0 {
		maxSize = 100
	}
	return &LRUMemoryCache{
		maxSize: maxSize,
		items:   make(map[string]string),
		order:   make([]string, 0, maxSize),
	}
}

// Get retrieves a cached value by key
func (c *LRUMemoryCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.items[key]
	return val, ok
}

// Set stores a value in the cache, evicting LRU items if needed
func (c *LRUMemoryCache) Set(key string, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if key already exists
	exists := false
	if _, ok := c.items[key]; ok {
		exists = true
		// Remove from order list
		for i, k := range c.order {
			if k == key {
				c.order = append(c.order[:i], c.order[i+1:]...)
				break
			}
		}
	}

	// If we're at capacity, evict the least recently used item
	if len(c.order) >= c.maxSize && !exists {
		if len(c.order) > 0 {
			oldestKey := c.order[0]
			delete(c.items, oldestKey)
			c.order = c.order[1:]
		}
	}

	c.items[key] = value
	c.order = append(c.order, key)
	return nil
}

// Clear removes all entries from the cache
func (c *LRUMemoryCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]string)
	c.order = make([]string, 0, c.maxSize)
	return nil
}

// HybridCache combines in-memory LRU cache with optional disk persistence
type HybridCache struct {
	memory   *LRUMemoryCache
	diskDir  string
	cacheTTL time.Duration
	mu       sync.RWMutex
}

// NewHybridCache creates a cache with both in-memory and disk persistence
// diskDir is the directory to store cached files; if empty, disk persistence is disabled
func NewHybridCache(memorySize int, diskDir string, ttl time.Duration) *HybridCache {
	// Create disk directory if specified
	if diskDir != "" {
		os.MkdirAll(diskDir, 0o700)
	}

	return &HybridCache{
		memory:   NewLRUMemoryCache(memorySize),
		diskDir:  diskDir,
		cacheTTL: ttl,
	}
}

// Get retrieves a cached value, checking memory first then disk
func (c *HybridCache) Get(key string) (string, bool) {
	// Check memory first
	if val, ok := c.memory.Get(key); ok {
		return val, true
	}

	// Check disk if directory is configured
	if c.diskDir == "" {
		return "", false
	}

	fileName := c.hashKeyToFileName(key)
	filePath := filepath.Join(c.diskDir, fileName)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", false
	}

	// Check TTL
	info, err := os.Stat(filePath)
	if err != nil {
		return "", false
	}

	if c.cacheTTL > 0 {
		age := time.Since(info.ModTime())
		if age > c.cacheTTL {
			// Cache expired, remove the file
			os.Remove(filePath)
			return "", false
		}
	}

	// Cache hit on disk - promote to memory
	val := string(data)
	c.memory.Set(key, val)
	return val, true
}

// Set stores a value in both memory and disk caches
func (c *HybridCache) Set(key string, value string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Always store in memory
	if err := c.memory.Set(key, value); err != nil {
		return err
	}

	// Store on disk if configured
	if c.diskDir == "" {
		return nil
	}

	fileName := c.hashKeyToFileName(key)
	filePath := filepath.Join(c.diskDir, fileName)

	if err := os.WriteFile(filePath, []byte(value), 0o600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Clear removes all entries from both memory and disk
func (c *HybridCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear memory
	if err := c.memory.Clear(); err != nil {
		return err
	}

	// Clear disk
	if c.diskDir == "" {
		return nil
	}

	return os.RemoveAll(c.diskDir)
}

// hashKeyToFileName converts a cache key to a safe filename using SHA256
func (c *HybridCache) hashKeyToFileName(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("cache_%x.dat", hash)
}

// GetCacheDir returns the default cache directory for the application
func GetCacheDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cache", "plexmusic-tui"), nil
}

package sharding

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"math/rand"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/samandartukhtayev/replication-and-sharding/config"
)

// ShardManager manages database shards and their replicas
type ShardManager struct {
	shards    []*Shard
	numShards int
	mu        sync.RWMutex
}

// Shard represents a single database shard with primary and replica connections
type Shard struct {
	ShardID  int
	Primary  *sql.DB
	Replicas []*sql.DB
}

// NewShardManager creates a new shard manager with the given configuration
func NewShardManager(cfg *config.Config) (*ShardManager, error) {
	sm := &ShardManager{
		shards:    make([]*Shard, len(cfg.Shards)),
		numShards: len(cfg.Shards),
	}

	// Initialize each shard with primary and replica connections
	for i, shardCfg := range cfg.Shards {
		shard := &Shard{
			ShardID:  shardCfg.ShardID,
			Replicas: make([]*sql.DB, 0),
		}

		// Connect to primary
		primaryDB, err := sql.Open("pgx", shardCfg.Primary.ConnectionString())
		if err != nil {
			return nil, fmt.Errorf("failed to connect to primary for shard %d: %w", shardCfg.ShardID, err)
		}

		// Test primary connection
		if err := primaryDB.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping primary for shard %d: %w", shardCfg.ShardID, err)
		}

		shard.Primary = primaryDB

		// Connect to replicas
		for j, replicaCfg := range shardCfg.Replicas {
			replicaDB, err := sql.Open("pgx", replicaCfg.ConnectionString())
			if err != nil {
				return nil, fmt.Errorf("failed to connect to replica %d for shard %d: %w", j, shardCfg.ShardID, err)
			}

			// Test replica connection
			if err := replicaDB.Ping(); err != nil {
				return nil, fmt.Errorf("failed to ping replica %d for shard %d: %w", j, shardCfg.ShardID, err)
			}

			shard.Replicas = append(shard.Replicas, replicaDB)
		}

		sm.shards[i] = shard
	}

	return sm, nil
}

// GetShardID calculates which shard a key belongs to using consistent hashing
// This is the core sharding logic - we use FNV hash for deterministic shard selection
func (sm *ShardManager) GetShardID(shardKey string) int {
	// Use FNV-1a hash function for good distribution
	h := fnv.New32a()
	h.Write([]byte(shardKey))
	hashValue := h.Sum32()

	// Modulo operation to map hash to a shard
	// This ensures the same key always goes to the same shard
	shardID := int(hashValue) % sm.numShards
	return shardID
}

// GetPrimaryDB returns the primary database for a given shard key
// All write operations should use this
func (sm *ShardManager) GetPrimaryDB(shardKey string) *sql.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shardID := sm.GetShardID(shardKey)
	return sm.shards[shardID].Primary
}

// GetReplicaDB returns a replica database for a given shard key
// Read operations can use this for load distribution
// If no replicas are available, it returns the primary
func (sm *ShardManager) GetReplicaDB(shardKey string) *sql.DB {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	shardID := sm.GetShardID(shardKey)
	shard := sm.shards[shardID]

	// If no replicas available, fall back to primary
	if len(shard.Replicas) == 0 {
		return shard.Primary
	}

	// Randomly select a replica for load balancing
	// In production, you might use round-robin or health-based selection
	replicaIdx := rand.Intn(len(shard.Replicas))
	return shard.Replicas[replicaIdx]
}

// GetShardByID returns a specific shard by its ID
// Useful for administrative operations or migrations
func (sm *ShardManager) GetShardByID(shardID int) (*Shard, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if shardID < 0 || shardID >= sm.numShards {
		return nil, fmt.Errorf("invalid shard ID: %d", shardID)
	}

	return sm.shards[shardID], nil
}

// GetAllShards returns all shards
// Useful for operations that need to run across all shards (e.g., migrations, analytics)
func (sm *ShardManager) GetAllShards() []*Shard {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Return a copy to prevent external modifications
	shardsCopy := make([]*Shard, len(sm.shards))
	copy(shardsCopy, sm.shards)
	return shardsCopy
}

// Close closes all database connections
func (sm *ShardManager) Close() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	var errs []error

	for _, shard := range sm.shards {
		if err := shard.Primary.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close primary for shard %d: %w", shard.ShardID, err))
		}

		for i, replica := range shard.Replicas {
			if err := replica.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close replica %d for shard %d: %w", i, shard.ShardID, err))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}

// NumShards returns the total number of shards
func (sm *ShardManager) NumShards() int {
	return sm.numShards
}

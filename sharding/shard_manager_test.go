package sharding

import (
	"testing"

	"github.com/samandartukhtayev/replication-and-sharding/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShardManager_GetShardID(t *testing.T) {
	cfg := config.DefaultConfig()
	sm, err := NewShardManager(cfg)
	require.NoError(t, err)
	defer sm.Close()

	tests := []struct {
		name     string
		shardKey string
	}{
		{"user_1", "user_1"},
		{"user_2", "user_2"},
		{"user_3", "user_3"},
		{"user_100", "user_100"},
		{"user_1000", "user_1000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the same key always returns the same shard
			shardID1 := sm.GetShardID(tt.shardKey)
			shardID2 := sm.GetShardID(tt.shardKey)

			assert.Equal(t, shardID1, shardID2, "Same key should always map to the same shard")
			assert.GreaterOrEqual(t, shardID1, 0, "Shard ID should be non-negative")
			assert.Less(t, shardID1, sm.NumShards(), "Shard ID should be less than number of shards")
		})
	}
}

func TestShardManager_ShardDistribution(t *testing.T) {
	cfg := config.DefaultConfig()
	sm, err := NewShardManager(cfg)
	require.NoError(t, err)
	defer sm.Close()

	// Test that keys are reasonably distributed across shards
	shardCounts := make(map[int]int)
	numKeys := 1000

	for i := 0; i < numKeys; i++ {
		key := "user_" + string(rune(i))
		shardID := sm.GetShardID(key)
		shardCounts[shardID]++
	}

	// Each shard should have at least some keys (not a perfect distribution, but reasonable)
	for shardID := 0; shardID < sm.NumShards(); shardID++ {
		count := shardCounts[shardID]
		t.Logf("Shard %d: %d keys (%.2f%%)", shardID, count, float64(count)/float64(numKeys)*100)
		assert.Greater(t, count, 0, "Each shard should have at least some keys")
	}
}

func TestShardManager_GetPrimaryDB(t *testing.T) {
	cfg := config.DefaultConfig()
	sm, err := NewShardManager(cfg)
	require.NoError(t, err)
	defer sm.Close()

	db := sm.GetPrimaryDB("test_user_123")
	assert.NotNil(t, db, "Should return a valid database connection")

	// Test connection is alive
	err = db.Ping()
	assert.NoError(t, err, "Primary database should be reachable")
}

func TestShardManager_GetReplicaDB(t *testing.T) {
	cfg := config.DefaultConfig()
	sm, err := NewShardManager(cfg)
	require.NoError(t, err)
	defer sm.Close()

	db := sm.GetReplicaDB("test_user_456")
	assert.NotNil(t, db, "Should return a valid database connection")

	// Test connection is alive
	err = db.Ping()
	assert.NoError(t, err, "Replica database should be reachable")
}

func TestShardManager_GetShardByID(t *testing.T) {
	cfg := config.DefaultConfig()
	sm, err := NewShardManager(cfg)
	require.NoError(t, err)
	defer sm.Close()

	// Test valid shard IDs
	for i := 0; i < sm.NumShards(); i++ {
		shard, err := sm.GetShardByID(i)
		assert.NoError(t, err)
		assert.NotNil(t, shard)
		assert.Equal(t, i, shard.ShardID)
	}

	// Test invalid shard ID
	_, err = sm.GetShardByID(-1)
	assert.Error(t, err)

	_, err = sm.GetShardByID(sm.NumShards())
	assert.Error(t, err)
}

func TestShardManager_GetAllShards(t *testing.T) {
	cfg := config.DefaultConfig()
	sm, err := NewShardManager(cfg)
	require.NoError(t, err)
	defer sm.Close()

	shards := sm.GetAllShards()
	assert.Len(t, shards, sm.NumShards())

	for i, shard := range shards {
		assert.Equal(t, i, shard.ShardID)
		assert.NotNil(t, shard.Primary)
		assert.NotEmpty(t, shard.Replicas)
	}
}

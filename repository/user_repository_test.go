package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/samandartukhtayev/replication-and-sharding/config"
	"github.com/samandartukhtayev/replication-and-sharding/models"
	"github.com/samandartukhtayev/replication-and-sharding/sharding"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestRepository(t *testing.T) (*UserRepository, *sharding.ShardManager, func()) {
	cfg := config.DefaultConfig()
	sm, err := sharding.NewShardManager(cfg)
	require.NoError(t, err)

	repo := NewUserRepository(sm)

	// Clean up any existing test data
	cleanupAllTestData(t, repo)

	// Return cleanup function
	cleanup := func() {
		cleanupAllTestData(t, repo)
		sm.Close()
	}

	return repo, sm, cleanup
}

func cleanupAllTestData(t *testing.T, repo *UserRepository) {
	ctx := context.Background()

	// Get all users from all shards and delete them
	allUsers, err := repo.GetAllUsers(ctx)
	if err == nil {
		for _, user := range allUsers {
			_ = repo.Delete(ctx, user.UserID)
		}
	}

	// Also try to delete specific test user IDs
	testUserIDs := []string{
		"test_user_1", "test_user_2", "test_user_3",
		"test_user_100", "test_user_200", "test_user_300",
		"test_user_delete",
		"count_user_1", "count_user_2", "count_user_3",
		"replication_test_user",
		"all_users_1", "all_users_2", "all_users_3",
	}

	for _, userID := range testUserIDs {
		_ = repo.Delete(ctx, userID)
	}

	// Wait for deletions to complete and replicate
	time.Sleep(200 * time.Millisecond)
}

func TestUserRepository_CreateAndGet(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{
		UserID: "test_user_1",
		Name:   "John Doe",
		Email:  "john@example.com",
	}

	// Create user
	err := repo.Create(ctx, user)
	require.NoError(t, err)
	assert.NotZero(t, user.ID, "User ID should be set after creation")
	assert.False(t, user.CreatedAt.IsZero(), "CreatedAt should be set")

	// Wait a moment for replication to catch up
	time.Sleep(100 * time.Millisecond)

	// Get user from replica
	retrieved, err := repo.GetByUserID(ctx, "test_user_1")
	require.NoError(t, err)
	assert.Equal(t, user.UserID, retrieved.UserID)
	assert.Equal(t, user.Name, retrieved.Name)
	assert.Equal(t, user.Email, retrieved.Email)

	// Clean up
	err = repo.Delete(ctx, user.UserID)
	require.NoError(t, err)
}

func TestUserRepository_GetByUserIDFromPrimary(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{
		UserID: "test_user_2",
		Name:   "Jane Smith",
		Email:  "jane@example.com",
	}

	// Create user
	err := repo.Create(ctx, user)
	require.NoError(t, err)

	// Immediately read from primary (no need to wait for replication)
	retrieved, err := repo.GetByUserIDFromPrimary(ctx, "test_user_2")
	require.NoError(t, err)
	assert.Equal(t, user.UserID, retrieved.UserID)
	assert.Equal(t, user.Name, retrieved.Name)
	assert.Equal(t, user.Email, retrieved.Email)

	// Clean up
	err = repo.Delete(ctx, user.UserID)
	require.NoError(t, err)
}

func TestUserRepository_Update(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{
		UserID: "test_user_3",
		Name:   "Bob Johnson",
		Email:  "bob@example.com",
	}

	// Create user
	err := repo.Create(ctx, user)
	require.NoError(t, err)

	// Update user
	user.Name = "Robert Johnson"
	user.Email = "robert@example.com"
	err = repo.Update(ctx, user)
	require.NoError(t, err)

	// Wait for replication
	time.Sleep(100 * time.Millisecond)

	// Verify update
	retrieved, err := repo.GetByUserID(ctx, user.UserID)
	require.NoError(t, err)
	assert.Equal(t, "Robert Johnson", retrieved.Name)
	assert.Equal(t, "robert@example.com", retrieved.Email)

	// Clean up
	err = repo.Delete(ctx, user.UserID)
	require.NoError(t, err)
}

func TestUserRepository_Delete(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{
		UserID: "test_user_delete",
		Name:   "Delete Me",
		Email:  "delete@example.com",
	}

	// Create user
	err := repo.Create(ctx, user)
	require.NoError(t, err)

	// Delete user
	err = repo.Delete(ctx, user.UserID)
	require.NoError(t, err)

	// Verify deletion
	_, err = repo.GetByUserIDFromPrimary(ctx, user.UserID)
	assert.Error(t, err, "User should not be found after deletion")
}

func TestUserRepository_ShardDistribution(t *testing.T) {
	repo, sm, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple users and verify they're distributed across shards
	users := []*models.User{
		{UserID: "test_user_100", Name: "User 100", Email: "user100@example.com"},
		{UserID: "test_user_200", Name: "User 200", Email: "user200@example.com"},
		{UserID: "test_user_300", Name: "User 300", Email: "user300@example.com"},
	}

	// Track which shard each user goes to
	shardMap := make(map[string]int)

	for _, user := range users {
		err := repo.Create(ctx, user)
		require.NoError(t, err)

		shardID := sm.GetShardID(user.UserID)
		shardMap[user.UserID] = shardID
		t.Logf("User %s -> Shard %d", user.UserID, shardID)
	}

	// Verify users can be retrieved from the correct shards
	for _, user := range users {
		retrieved, err := repo.GetByUserIDFromPrimary(ctx, user.UserID)
		require.NoError(t, err)
		assert.Equal(t, user.UserID, retrieved.UserID)

		// Verify the shard ID is consistent
		expectedShardID := shardMap[user.UserID]
		actualShardID := sm.GetShardID(user.UserID)
		assert.Equal(t, expectedShardID, actualShardID, "Shard ID should be consistent")
	}

	// Clean up
	for _, user := range users {
		err := repo.Delete(ctx, user.UserID)
		require.NoError(t, err)
	}
}

func TestUserRepository_CountUsersPerShard(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Verify database is clean before test
	initialCounts, err := repo.CountUsersPerShard(ctx)
	require.NoError(t, err)
	initialTotal := 0
	for _, count := range initialCounts {
		initialTotal += count
	}
	t.Logf("Initial total users in database: %d", initialTotal)

	// Create users
	users := []*models.User{
		{UserID: "count_user_1", Name: "User 1", Email: "user1@example.com"},
		{UserID: "count_user_2", Name: "User 2", Email: "user2@example.com"},
		{UserID: "count_user_3", Name: "User 3", Email: "user3@example.com"},
	}

	for _, user := range users {
		err := repo.Create(ctx, user)
		require.NoError(t, err)
	}

	// Wait for data to be written
	time.Sleep(200 * time.Millisecond)

	// Count users per shard
	counts, err := repo.CountUsersPerShard(ctx)
	require.NoError(t, err)

	totalCount := 0
	for shardID, count := range counts {
		t.Logf("Shard %d has %d users", shardID, count)
		totalCount += count
	}

	// Assert only the users we created are counted
	expectedTotal := initialTotal + len(users)
	assert.Equal(t, expectedTotal, totalCount, "Total count should match initial + created users")

	// Clean up
	for _, user := range users {
		err := repo.Delete(ctx, user.UserID)
		require.NoError(t, err)
	}
}

func TestUserRepository_Replication(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	user := &models.User{
		UserID: "replication_test_user",
		Name:   "Replication Test",
		Email:  "replication@example.com",
	}

	// Create user (write to primary)
	err := repo.Create(ctx, user)
	require.NoError(t, err)
	t.Logf("User created with ID: %d", user.ID)

	// Immediately read from primary - should succeed
	primaryUser, err := repo.GetByUserIDFromPrimary(ctx, user.UserID)
	require.NoError(t, err)
	assert.Equal(t, user.UserID, primaryUser.UserID)
	t.Logf("Successfully read from primary immediately after write")

	// Wait for replication to propagate
	t.Log("Waiting for replication to propagate...")
	time.Sleep(200 * time.Millisecond)

	// Read from replica - should now succeed
	replicaUser, err := repo.GetByUserID(ctx, user.UserID)
	require.NoError(t, err)
	assert.Equal(t, user.UserID, replicaUser.UserID)
	assert.Equal(t, user.Name, replicaUser.Name)
	assert.Equal(t, user.Email, replicaUser.Email)
	t.Logf("Successfully read from replica after replication")

	// Verify data consistency between primary and replica
	assert.Equal(t, primaryUser.ID, replicaUser.ID, "IDs should match")
	assert.Equal(t, primaryUser.Name, replicaUser.Name, "Names should match")
	assert.Equal(t, primaryUser.Email, replicaUser.Email, "Emails should match")

	// Clean up
	err = repo.Delete(ctx, user.UserID)
	require.NoError(t, err)
}

func TestUserRepository_GetAllUsers(t *testing.T) {
	repo, _, cleanup := setupTestRepository(t)
	defer cleanup()

	ctx := context.Background()

	// Create users across different shards
	users := []*models.User{
		{UserID: "all_users_1", Name: "User 1", Email: "user1@example.com"},
		{UserID: "all_users_2", Name: "User 2", Email: "user2@example.com"},
		{UserID: "all_users_3", Name: "User 3", Email: "user3@example.com"},
	}

	for _, user := range users {
		err := repo.Create(ctx, user)
		require.NoError(t, err)
	}

	// Wait for replication
	time.Sleep(200 * time.Millisecond)

	// Get all users
	allUsers, err := repo.GetAllUsers(ctx)
	require.NoError(t, err)

	// Verify we got at least our test users
	userMap := make(map[string]*models.User)
	for _, user := range allUsers {
		userMap[user.UserID] = user
	}

	for _, expectedUser := range users {
		actualUser, found := userMap[expectedUser.UserID]
		assert.True(t, found, fmt.Sprintf("User %s should be in results", expectedUser.UserID))
		if found {
			assert.Equal(t, expectedUser.Name, actualUser.Name)
			assert.Equal(t, expectedUser.Email, actualUser.Email)
		}
	}

	// Clean up
	for _, user := range users {
		err := repo.Delete(ctx, user.UserID)
		require.NoError(t, err)
	}
}

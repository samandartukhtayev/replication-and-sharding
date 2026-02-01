package main

import (
	"context"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
	"github.com/samandartukhtayev/replication-and-sharding/config"
	"github.com/samandartukhtayev/replication-and-sharding/models"
	"github.com/samandartukhtayev/replication-and-sharding/repository"
	"github.com/samandartukhtayev/replication-and-sharding/sharding"
)

func main() {
	fmt.Println("=== Database Sharding and Replication Demo ===")

	// Load configuration
	cfg := config.DefaultConfig()
	fmt.Printf("Initialized with %d shards\n", len(cfg.Shards))

	// Create shard manager
	sm, err := sharding.NewShardManager(cfg)
	if err != nil {
		log.Fatalf("Failed to create shard manager: %v", err)
	}
	defer sm.Close()

	fmt.Println("✓ Connected to all database shards and replicas")

	// Create repository
	repo := repository.NewUserRepository(sm)

	// Demonstrate sharding
	demonstrateSharding(sm)

	// Demonstrate CRUD operations
	demonstrateCRUD(repo, sm)

	// Demonstrate replication
	demonstrateReplication(repo, sm)

	// Show shard distribution
	demonstrateShardDistribution(repo)

	fmt.Println("\n=== Demo Complete ===")
}

func demonstrateSharding(sm *sharding.ShardManager) {
	fmt.Println("--- Sharding Demonstration ---")

	userIDs := []string{"user_alice", "user_bob", "user_charlie", "user_diana", "user_eve"}

	fmt.Println("Shard key distribution:")
	for _, userID := range userIDs {
		shardID := sm.GetShardID(userID)
		fmt.Printf("  %s -> Shard %d\n", userID, shardID)
	}
	fmt.Println()
}

func demonstrateCRUD(repo *repository.UserRepository, sm *sharding.ShardManager) {
	fmt.Println("--- CRUD Operations Demonstration ---")
	ctx := context.Background()

	// Create
	user := &models.User{
		UserID: "demo_user_123",
		Name:   "Alice Johnson",
		Email:  "alice@example.com",
	}

	shardID := sm.GetShardID(user.UserID)
	fmt.Printf("Creating user '%s' in Shard %d...\n", user.UserID, shardID)

	err := repo.Create(ctx, user)
	if err != nil {
		log.Printf("Error creating user: %v", err)
		return
	}
	fmt.Printf("✓ User created with ID: %d\n", user.ID)

	// Read from primary immediately
	fmt.Println("Reading from PRIMARY (immediate consistency)...")
	retrieved, err := repo.GetByUserIDFromPrimary(ctx, user.UserID)
	if err != nil {
		log.Printf("Error reading user: %v", err)
	} else {
		fmt.Printf("✓ Found: %s (%s)\n", retrieved.Name, retrieved.Email)
	}

	// Wait for replication
	fmt.Println("Waiting for replication to propagate...")
	time.Sleep(200 * time.Millisecond)

	// Read from replica
	fmt.Println("Reading from REPLICA...")
	retrieved, err = repo.GetByUserID(ctx, user.UserID)
	if err != nil {
		log.Printf("Error reading user from replica: %v", err)
	} else {
		fmt.Printf("✓ Found in replica: %s (%s)\n", retrieved.Name, retrieved.Email)
	}

	// Update
	fmt.Println("\nUpdating user...")
	user.Name = "Alice Smith"
	user.Email = "alice.smith@example.com"
	err = repo.Update(ctx, user)
	if err != nil {
		log.Printf("Error updating user: %v", err)
	} else {
		fmt.Println("✓ User updated")
	}

	// Wait for replication
	time.Sleep(200 * time.Millisecond)

	// Verify update
	retrieved, err = repo.GetByUserID(ctx, user.UserID)
	if err != nil {
		log.Printf("Error reading updated user: %v", err)
	} else {
		fmt.Printf("✓ Updated data in replica: %s (%s)\n", retrieved.Name, retrieved.Email)
	}

	// Delete
	fmt.Println("\nDeleting user...")
	err = repo.Delete(ctx, user.UserID)
	if err != nil {
		log.Printf("Error deleting user: %v", err)
	} else {
		fmt.Println("✓ User deleted")
	}
	fmt.Println()
}

func demonstrateReplication(repo *repository.UserRepository, sm *sharding.ShardManager) {
	fmt.Println("--- Replication Demonstration ---")
	ctx := context.Background()

	user := &models.User{
		UserID: "replication_demo_user",
		Name:   "Bob Wilson",
		Email:  "bob@example.com",
	}

	shardID := sm.GetShardID(user.UserID)
	fmt.Printf("Creating user in Shard %d...\n", shardID)

	// Write to primary
	err := repo.Create(ctx, user)
	if err != nil {
		log.Printf("Error creating user: %v", err)
		return
	}
	fmt.Println("✓ Written to PRIMARY database")

	// Immediately try to read from primary
	fmt.Println("\nAttempting immediate read from PRIMARY...")
	primaryUser, err := repo.GetByUserIDFromPrimary(ctx, user.UserID)
	if err != nil {
		fmt.Printf("✗ Could not read from primary: %v\n", err)
	} else {
		fmt.Printf("✓ Successfully read from primary: %s\n", primaryUser.Name)
	}

	// Try to read from replica (might fail if replication hasn't completed)
	fmt.Println("\nAttempting immediate read from REPLICA...")
	_, err = repo.GetByUserID(ctx, user.UserID)
	if err != nil {
		fmt.Println("✗ Not yet available in replica (replication lag)")
	} else {
		fmt.Println("✓ Successfully read from replica (replication was fast!)")
	}

	// Wait for replication
	fmt.Println("\nWaiting 200ms for replication to complete...")
	time.Sleep(200 * time.Millisecond)

	// Read from replica again
	fmt.Println("Attempting read from REPLICA after waiting...")
	replicaUser, err := repo.GetByUserID(ctx, user.UserID)
	if err != nil {
		fmt.Printf("✗ Still not available: %v\n", err)
	} else {
		fmt.Printf("✓ Successfully read from replica: %s\n", replicaUser.Name)
		fmt.Println("✓ Replication confirmed working!")
	}

	// Clean up
	repo.Delete(ctx, user.UserID)
	fmt.Println()
}

func demonstrateShardDistribution(repo *repository.UserRepository) {
	fmt.Println("--- Shard Distribution Statistics ---")
	ctx := context.Background()

	// Create some test users
	testUsers := []struct {
		userID string
		name   string
	}{
		{"dist_user_1", "User 1"},
		{"dist_user_2", "User 2"},
		{"dist_user_3", "User 3"},
		{"dist_user_4", "User 4"},
		{"dist_user_5", "User 5"},
	}

	fmt.Println("Creating test users...")
	for _, tu := range testUsers {
		user := &models.User{
			UserID: tu.userID,
			Name:   tu.name,
			Email:  tu.userID + "@example.com",
		}
		repo.Create(ctx, user)
	}

	// Get counts
	counts, err := repo.CountUsersPerShard(ctx)
	if err != nil {
		log.Printf("Error getting shard counts: %v", err)
		return
	}

	fmt.Println("\nUsers per shard:")
	totalUsers := 0
	for shardID, count := range counts {
		fmt.Printf("  Shard %d: %d users\n", shardID, count)
		totalUsers += count
	}
	fmt.Printf("  Total: %d users\n", totalUsers)

	// Clean up
	for _, tu := range testUsers {
		repo.Delete(ctx, tu.userID)
	}
	fmt.Println()
}

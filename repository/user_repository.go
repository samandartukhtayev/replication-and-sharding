package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/samandartukhtayev/replication-and-sharding/models"
	"github.com/samandartukhtayev/replication-and-sharding/sharding"
)

// UserRepository handles all user-related database operations
// It abstracts away the sharding and replication complexity from the application layer
type UserRepository struct {
	shardManager *sharding.ShardManager
}

// NewUserRepository creates a new user repository
func NewUserRepository(sm *sharding.ShardManager) *UserRepository {
	return &UserRepository{
		shardManager: sm,
	}
}

// Create creates a new user
// Writes always go to the primary database of the appropriate shard
func (r *UserRepository) Create(ctx context.Context, user *models.User) error {
	// Determine which shard to write to based on the shard key (user_id)
	db := r.shardManager.GetPrimaryDB(user.UserID)

	query := `
		INSERT INTO users (user_id, name, email, created_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		RETURNING id, created_at
	`

	err := db.QueryRowContext(ctx, query, user.UserID, user.Name, user.Email).
		Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByUserID retrieves a user by their user_id
// Reads can come from replica databases for better load distribution
func (r *UserRepository) GetByUserID(ctx context.Context, userID string) (*models.User, error) {
	// Read from replica to reduce load on primary
	db := r.shardManager.GetReplicaDB(userID)

	query := `
		SELECT id, user_id, name, email, created_at
		FROM users
		WHERE user_id = $1
	`

	user := &models.User{}
	err := db.QueryRowContext(ctx, query, userID).
		Scan(&user.ID, &user.UserID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetByUserIDFromPrimary retrieves a user from the primary database
// Use this when you need the most up-to-date data (e.g., after a write)
func (r *UserRepository) GetByUserIDFromPrimary(ctx context.Context, userID string) (*models.User, error) {
	// Read from primary for strong consistency
	db := r.shardManager.GetPrimaryDB(userID)

	query := `
		SELECT id, user_id, name, email, created_at
		FROM users
		WHERE user_id = $1
	`

	user := &models.User{}
	err := db.QueryRowContext(ctx, query, userID).
		Scan(&user.ID, &user.UserID, &user.Name, &user.Email, &user.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found: %s", userID)
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// Update updates an existing user
// Writes always go to the primary database
func (r *UserRepository) Update(ctx context.Context, user *models.User) error {
	db := r.shardManager.GetPrimaryDB(user.UserID)

	query := `
		UPDATE users
		SET name = $1, email = $2
		WHERE user_id = $3
	`

	result, err := db.ExecContext(ctx, query, user.Name, user.Email, user.UserID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found: %s", user.UserID)
	}

	return nil
}

// Delete deletes a user by their user_id
// Writes always go to the primary database
func (r *UserRepository) Delete(ctx context.Context, userID string) error {
	db := r.shardManager.GetPrimaryDB(userID)

	query := `DELETE FROM users WHERE user_id = $1`

	result, err := db.ExecContext(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("user not found: %s", userID)
	}

	return nil
}

// GetAllUsers retrieves all users across all shards
// This is an expensive operation as it queries all shards
// Use pagination in production scenarios
func (r *UserRepository) GetAllUsers(ctx context.Context) ([]*models.User, error) {
	shards := r.shardManager.GetAllShards()
	var allUsers []*models.User

	query := `
		SELECT id, user_id, name, email, created_at
		FROM users
		ORDER BY created_at DESC
	`

	// Query each shard
	for _, shard := range shards {
		// Use replica for reads
		db := shard.Replicas[0]
		if db == nil {
			db = shard.Primary
		}

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to query shard %d: %w", shard.ShardID, err)
		}

		for rows.Next() {
			user := &models.User{}
			if err := rows.Scan(&user.ID, &user.UserID, &user.Name, &user.Email, &user.CreatedAt); err != nil {
				rows.Close()
				return nil, fmt.Errorf("failed to scan user from shard %d: %w", shard.ShardID, err)
			}
			allUsers = append(allUsers, user)
		}

		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating rows from shard %d: %w", shard.ShardID, err)
		}
	}

	return allUsers, nil
}

// CountUsersPerShard returns the count of users in each shard
// Useful for monitoring shard distribution
func (r *UserRepository) CountUsersPerShard(ctx context.Context) (map[int]int, error) {
	shards := r.shardManager.GetAllShards()
	counts := make(map[int]int)

	query := `SELECT COUNT(*) FROM users`

	for _, shard := range shards {
		var count int
		err := shard.Primary.QueryRowContext(ctx, query).Scan(&count)
		if err != nil {
			return nil, fmt.Errorf("failed to count users in shard %d: %w", shard.ShardID, err)
		}
		counts[shard.ShardID] = count
	}

	return counts, nil
}

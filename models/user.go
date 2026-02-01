package models

import "time"

// User represents a user in our system
type User struct {
	ID        int       `json:"id"`
	UserID    string    `json:"user_id"`    // This is the shard key
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}
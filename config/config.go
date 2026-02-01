package config

import (
	"fmt"
)

// ShardConfig represents configuration for a single shard
type ShardConfig struct {
	ShardID  int
	Primary  DatabaseConfig
	Replicas []DatabaseConfig
}

// DatabaseConfig represents a single database connection configuration
type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

// Config holds the complete application configuration
type Config struct {
	Shards []ShardConfig
}

// ConnectionString returns a PostgreSQL connection string
func (dc *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dc.Host, dc.Port, dc.User, dc.Password, dc.DBName,
	)
}

// DefaultConfig returns the default configuration with 3 shards and 1 replica each
func DefaultConfig() *Config {
	return &Config{
		Shards: []ShardConfig{
			{
				ShardID: 0,
				Primary: DatabaseConfig{
					Host:     "localhost",
					Port:     5440,
					User:     "postgres",
					Password: "postgres",
					DBName:   "shard0",
				},
				Replicas: []DatabaseConfig{
					{
						Host:     "localhost",
						Port:     5441,
						User:     "postgres",
						Password: "postgres",
						DBName:   "shard0",
					},
				},
			},
			{
				ShardID: 1,
				Primary: DatabaseConfig{
					Host:     "localhost",
					Port:     5442,
					User:     "postgres",
					Password: "postgres",
					DBName:   "shard1",
				},
				Replicas: []DatabaseConfig{
					{
						Host:     "localhost",
						Port:     5443,
						User:     "postgres",
						Password: "postgres",
						DBName:   "shard1",
					},
				},
			},
			{
				ShardID: 2,
				Primary: DatabaseConfig{
					Host:     "localhost",
					Port:     5444,
					User:     "postgres",
					Password: "postgres",
					DBName:   "shard2",
				},
				Replicas: []DatabaseConfig{
					{
						Host:     "localhost",
						Port:     5445,
						User:     "postgres",
						Password: "postgres",
						DBName:   "shard2",
					},
				},
			},
		},
	}
}

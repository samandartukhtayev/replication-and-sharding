#!/bin/bash
set -e

echo "Starting replication initialization for shard ${SHARD_ID}..."

# Create replication user
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Create replication user if not exists
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_user WHERE usename = 'replicator') THEN
            CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'replicator_password';
        END IF;
    END
    \$\$;
EOSQL

# Create replication slot based on SHARD_ID environment variable
if [ ! -z "$SHARD_ID" ]; then
    psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
        -- Create replication slot if not exists
        DO \$\$
        BEGIN
            IF NOT EXISTS (
                SELECT 1 FROM pg_replication_slots 
                WHERE slot_name = 'replication_slot_${SHARD_ID}'
            ) THEN
                PERFORM pg_create_physical_replication_slot('replication_slot_${SHARD_ID}');
            END IF;
        END
        \$\$;
EOSQL
    echo "Created replication slot: replication_slot_${SHARD_ID}"
fi

# Update pg_hba.conf to allow replication connections
echo "host replication replicator 0.0.0.0/0 md5" >> "$PGDATA/pg_hba.conf"

# Create application schema
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    CREATE TABLE IF NOT EXISTS users (
        id SERIAL PRIMARY KEY,
        user_id VARCHAR(255) NOT NULL UNIQUE,
        name VARCHAR(255) NOT NULL,
        email VARCHAR(255) NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    );

    CREATE INDEX IF NOT EXISTS idx_users_user_id ON users(user_id);
    
    -- Insert sample data for testing
    INSERT INTO users (user_id, name, email) 
    VALUES 
        ('user_shard${SHARD_ID}_1', 'Test User 1', 'test1@shard${SHARD_ID}.com'),
        ('user_shard${SHARD_ID}_2', 'Test User 2', 'test2@shard${SHARD_ID}.com')
    ON CONFLICT (user_id) DO NOTHING;
EOSQL

echo "Replication setup completed successfully for shard ${SHARD_ID}"
echo "Database: $POSTGRES_DB"
echo "Replication slot: replication_slot_${SHARD_ID}"
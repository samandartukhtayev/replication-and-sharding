# Testing Guide: Database Sharding and Replication

This guide explains how the tests work and what they verify.

## Test Architecture

```
Tests
├── Unit Tests (sharding/)
│   └── Test shard selection logic in isolation
└── Integration Tests (repository/)
    └── Test complete system with real databases
```

## Running Tests

### Quick Start

```bash
# Start databases first
make start

# Run all tests
make test

# Run with verbose output
make test-verbose
```

### Individual Test Suites

```bash
# Test sharding logic only
go test ./sharding -v

# Test repository operations only
go test ./repository -v

# Run specific test
go test ./repository -v -run TestUserRepository_Replication
```

## Unit Tests Explained

### Test: GetShardID Consistency

**File**: `sharding/shard_manager_test.go`

**What it tests**: The same shard key always maps to the same shard

```go
func TestShardManager_GetShardID(t *testing.T)
```

**How it works**:
1. Create a shard manager with 3 shards
2. Hash the same key multiple times
3. Verify we always get the same shard ID

**Why it's important**: Consistent hashing is fundamental to sharding. If a key maps to different shards at different times, you'll lose data or create duplicates.

**Example**:
```
First call:  sm.GetShardID("user_123")  → Shard 2
Second call: sm.GetShardID("user_123")  → Shard 2  ✓ SAME
```

### Test: Shard Distribution

**File**: `sharding/shard_manager_test.go`

**What it tests**: Keys are reasonably distributed across all shards

```go
func TestShardManager_ShardDistribution(t *testing.T)
```

**How it works**:
1. Hash 1000 different keys
2. Count how many keys go to each shard
3. Verify each shard gets at least some keys

**Why it's important**: Poor distribution leads to "hot" shards (overloaded) and "cold" shards (underutilized).

**Example output**:
```
Shard 0: 337 keys (33.70%)
Shard 1: 329 keys (32.90%)
Shard 2: 334 keys (33.40%)
```

## Integration Tests Explained

### Test: Create and Get

**File**: `repository/user_repository_test.go`

**What it tests**: Basic CRUD operations work with sharding

```go
func TestUserRepository_CreateAndGet(t *testing.T)
```

**How it works**:
1. Create a user (write to primary)
2. Wait for replication (100ms)
3. Read user from replica
4. Verify data matches

**Why it's important**: Verifies the basic flow of write → replicate → read works correctly.

**Timeline**:
```
T=0ms:    Create user → Primary DB
T=0-100ms: Replication happens
T=100ms:  Read user ← Replica DB
```

### Test: Replication

**File**: `repository/user_repository_test.go`

**What it tests**: Data written to primary appears in replica

```go
func TestUserRepository_Replication(t *testing.T)
```

**How it works**:
1. Write user to primary
2. Immediately read from primary (should succeed)
3. Immediately read from replica (might fail - replication lag)
4. Wait 200ms
5. Read from replica again (should succeed)

**Why it's important**: This is the core test proving replication actually works. It demonstrates:
- Primary writes are immediate
- Replica reads have eventual consistency
- Replication lag is measurable and bounded

**Expected behavior**:
```
Write to primary           ✓ Success
Read from primary (T=0ms)  ✓ Success (no lag)
Read from replica (T=0ms)  ✗ May fail (replication lag)
Read from replica (T=200ms) ✓ Success (replicated)
```

### Test: Shard Distribution

**File**: `repository/user_repository_test.go`

**What it tests**: Users with different IDs go to different shards

```go
func TestUserRepository_ShardDistribution(t *testing.T)
```

**How it works**:
1. Create 3 users with different IDs
2. Track which shard each user goes to
3. Verify shard selection is consistent
4. Verify users can be retrieved from their shards

**Why it's important**: Proves that the sharding mechanism actually routes data to different physical databases.

**Example**:
```
user_100 → Shard 1 ✓
user_200 → Shard 0 ✓
user_300 → Shard 2 ✓
All shards used ✓
```

### Test: Write to Primary

**File**: `repository/user_repository_test.go`

**What it tests**: Write operations always hit the primary database

```go
func TestUserRepository_GetByUserIDFromPrimary(t *testing.T)
```

**How it works**:
1. Create a user
2. Immediately read from primary (no wait)
3. Verify data is available

**Why it's important**: Confirms that writes go to the primary and that you can read your own writes immediately from the primary (strong consistency).

**Benefit over replica reads**: No replication lag, guaranteed consistency.

### Test: Read from Replica

**File**: Multiple tests using `GetByUserID()`

**What it tests**: Read operations can use replica databases

**How it works**:
1. Create a user
2. Wait for replication
3. Call `GetByUserID()` (internally uses `GetReplicaDB()`)
4. Verify data is found

**Why it's important**: Proves that replicas can serve read traffic, reducing load on the primary.

## Test Data Flow

### Write Path

```
Test
  │
  │ repo.Create(user)
  │
  ▼
ShardManager.GetPrimaryDB(user.UserID)
  │
  │ Hash user_id → Shard ID
  │
  ▼
Primary Database (e.g., Shard 1 Primary on :5434)
  │
  │ INSERT INTO users ...
  │
  ▼
Write succeeds
  │
  │ Streaming Replication
  │
  ▼
Replica Database (e.g., Shard 1 Replica on :5435)
```

### Read Path

```
Test
  │
  │ repo.GetByUserID(user.UserID)
  │
  ▼
ShardManager.GetReplicaDB(user.UserID)
  │
  │ Hash user_id → Shard ID
  │ Select random replica
  │
  ▼
Replica Database
  │
  │ SELECT * FROM users WHERE user_id = ?
  │
  ▼
Return user data
```

## Common Test Patterns

### Pattern 1: Test with Cleanup

```go
func TestSomething(t *testing.T) {
    // Setup
    user := createTestUser()
    
    // Test
    err := repo.Create(ctx, user)
    require.NoError(t, err)
    
    // Verify
    retrieved, err := repo.GetByUserID(ctx, user.UserID)
    assert.Equal(t, user.Name, retrieved.Name)
    
    // Cleanup
    repo.Delete(ctx, user.UserID)
}
```

### Pattern 2: Wait for Replication

```go
// Write to primary
repo.Create(ctx, user)

// Wait for replication to complete
time.Sleep(100 * time.Millisecond)

// Now safe to read from replica
retrieved, _ := repo.GetByUserID(ctx, user.UserID)
```

### Pattern 3: Verify Sharding

```go
// Create users
users := []string{"alice", "bob", "charlie"}

// Track shards
shardMap := make(map[string]int)
for _, userID := range users {
    shardID := sm.GetShardID(userID)
    shardMap[userID] = shardID
}

// Verify consistency
for _, userID := range users {
    newShardID := sm.GetShardID(userID)
    assert.Equal(t, shardMap[userID], newShardID)
}
```

## Debugging Failed Tests

### Test fails with "connection refused"

**Cause**: Databases aren't running

**Solution**:
```bash
docker-compose up -d
sleep 10  # Wait for databases to be ready
go test ./...
```

### Test fails with "user not found" in replica

**Cause**: Replication lag is longer than expected

**Solution**: Increase sleep duration
```go
// Before
time.Sleep(100 * time.Millisecond)

// After
time.Sleep(500 * time.Millisecond)
```

### Test fails inconsistently

**Cause**: Race condition or replication timing

**Solution**: Use `GetByUserIDFromPrimary()` for deterministic reads
```go
// Instead of this (may fail due to replication lag)
user, err := repo.GetByUserID(ctx, userID)

// Use this (always consistent)
user, err := repo.GetByUserIDFromPrimary(ctx, userID)
```

## Writing Your Own Tests

### Test Template

```go
func TestYourFeature(t *testing.T) {
    // 1. Setup repository
    repo, sm := setupTestRepository(t)
    defer sm.Close()
    
    ctx := context.Background()
    
    // 2. Create test data
    user := &models.User{
        UserID: "test_" + t.Name(),  // Unique per test
        Name:   "Test User",
        Email:  "test@example.com",
    }
    
    // 3. Execute operation
    err := repo.Create(ctx, user)
    require.NoError(t, err)
    
    // 4. Verify result
    retrieved, err := repo.GetByUserIDFromPrimary(ctx, user.UserID)
    require.NoError(t, err)
    assert.Equal(t, user.Name, retrieved.Name)
    
    // 5. Cleanup
    err = repo.Delete(ctx, user.UserID)
    require.NoError(t, err)
}
```

### Best Practices

1. **Always cleanup**: Delete test data to avoid interference
2. **Use unique IDs**: Prevent conflicts between parallel tests
3. **Test one thing**: Each test should verify a single behavior
4. **Wait for replication**: Use sleep when testing replica reads
5. **Check errors**: Use `require.NoError()` for operations that must succeed

## Test Coverage

Run tests with coverage:

```bash
go test ./... -cover

# Generate HTML coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Performance Testing

### Benchmark Shard Selection

```go
func BenchmarkGetShardID(b *testing.B) {
    cfg := config.DefaultConfig()
    sm, _ := NewShardManager(cfg)
    defer sm.Close()
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        sm.GetShardID("user_" + strconv.Itoa(i))
    }
}
```

Run benchmarks:
```bash
go test ./sharding -bench=. -benchmem
```

## Continuous Integration

### GitHub Actions Example

```yaml
name: Tests

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v2
    
    - name: Set up Go
      uses: actions/setup-go@v2
      with:
        go-version: 1.21
    
    - name: Start databases
      run: docker-compose up -d
    
    - name: Wait for databases
      run: sleep 30
    
    - name: Run tests
      run: go test ./... -v
    
    - name: Stop databases
      run: docker-compose down
```

## Summary

The test suite verifies:

✅ **Sharding**
- Keys consistently map to the same shard
- Keys are distributed across all shards
- Shard selection is deterministic

✅ **Replication**  
- Writes go to primary databases
- Data replicates to replicas
- Replicas can serve read traffic
- Replication lag is bounded

✅ **Repository Operations**
- CRUD operations work correctly
- Data is stored in the correct shard
- Reads can come from replicas
- Updates and deletes work across shards

Run the tests frequently during development to catch issues early!
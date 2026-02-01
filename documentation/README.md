# Architecture Documentation

## Overview

This document describes a **production-grade PostgreSQL sharding architecture** implemented at the application layer, with **primary–replica replication per shard**, deterministic shard routing, and read/write separation. The design prioritizes **scalability, operational clarity, and predictable consistency trade-offs**.

The system is implemented in Go and follows a layered architecture:

* **Application layer** orchestrates requests
* **Repository layer** encapsulates data access
* **Sharding layer** determines shard placement and routing
* **PostgreSQL layer** provides persistence with streaming replication

---

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Application Layer                       │
│                                                             │
│   main.go  ──▶  Repository  ──▶  ShardManager               │
│                                                             │
└─────────────────────────────────────────────────────────────┘
                                  │
                ┌─────────────────┼─────────────────┐
                ▼                 ▼                 ▼
        ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
        │   Shard 0    │  │   Shard 1    │  │   Shard 2    │
        │ Primary +    │  │ Primary +    │  │ Primary +    │
        │ Replicas     │  │ Replicas     │  │ Replicas     │
        └──────────────┘  └──────────────┘  └──────────────┘
```

Each shard is an **independent PostgreSQL cluster** consisting of:

* One **primary** (handles all writes)
* One or more **replicas** (handle read traffic)

There is **no cross-shard replication**.

---

## Configuration Layer (`config/`)

### Responsibility

The configuration layer provides **static, explicit shard topology configuration**. Shards are not auto-discovered; all shard membership is application-controlled.

### Structure

```
Config
 └─ Shards []ShardConfig
     ├─ ShardID int
     ├─ Primary DatabaseConfig
     └─ Replicas []DatabaseConfig

DatabaseConfig
 ├─ Host string
 ├─ Port int
 ├─ User string
 ├─ Password string
 └─ DBName string
```

### Design Notes

* Configuration is loaded at startup
* Shard count is fixed for the lifetime of the process
* Adding shards requires a **redeploy** and **data migration**

---

## Sharding Layer (`sharding/`)

### Responsibility

The sharding layer performs **deterministic shard routing** based on a shard key. It is completely stateless after initialization.

### Core Abstractions

```
ShardManager
 ├─ shards []*Shard
 ├─ numShards int
 ├─ GetShardID(key string) int
 ├─ GetPrimaryDB(key string) *sql.DB
 ├─ GetReplicaDB(key string) *sql.DB
 └─ GetAllShards() []*Shard

Shard
 ├─ ID int
 ├─ Primary *sql.DB
 └─ Replicas []*sql.DB
```

### Routing Rules

* **Writes** → primary of the computed shard
* **Reads** → replica of the computed shard (or primary if replicas unavailable)
* **Cross-shard operations** must explicitly iterate over all shards

---

## Hashing Strategy

### Algorithm: FNV-1a (32-bit)

The shard ID is computed as:

```
shardID = fnv1a(shardKey) % numShards
```

### Rationale

* Fast and non-cryptographic
* Deterministic and stable
* Uniform distribution for typical user IDs

### Constraints

* Changing `numShards` **re-maps all keys**
* Native modulo hashing **does not support online resharding**

> In production systems that require online scaling, **consistent hashing or virtual shards** are recommended.

---

## Repository Layer (`repository/`)

### Responsibility

The repository layer encapsulates **all database access** and enforces read/write routing rules.

### Example: UserRepository

```
Create(user)                     → primary
GetByUserID(userID)              → replica
GetByUserIDFromPrimary(userID)   → primary
Update(user)                     → primary
Delete(userID)                   → primary
```

### Design Principles

* No SQL outside repositories
* No shard logic in business code
* Explicit primary reads where strong consistency is required

---

## Read & Write Flows

### Write Flow

1. Application receives request
2. Repository computes shard using `user_id`
3. ShardManager returns primary connection
4. Transaction executes on primary
5. WAL is generated and streamed to replicas asynchronously

Writes are **acknowledged before replicas apply WAL**.

---

### Read Flow (Replica)

1. Application issues read request
2. Repository computes shard
3. ShardManager selects a replica (round-robin or random)
4. Query executes on replica

Reads are **eventually consistent**.

---

## Replication Model

### PostgreSQL Streaming Replication

* Physical (WAL-based) replication
* Asynchronous by default
* Replicas are **read-only**

```
Primary
  └─ WAL Writer → WAL Sender ───▶ Replica
                                   └─ WAL Receiver → WAL Replay
```

### Consistency Guarantees

| Mode     | Read Target | Consistency                 |
| -------- | ----------- | --------------------------- |
| Strong   | Primary     | Read-after-write guaranteed |
| Eventual | Replica     | Subject to replication lag  |

Replication lag is typically **single-digit milliseconds** but not bounded.

---

## Database Schema

### Users Table

```sql
CREATE TABLE users (
    id          BIGSERIAL PRIMARY KEY,
    user_id     VARCHAR(255) NOT NULL,
    name        VARCHAR(255) NOT NULL,
    email       VARCHAR(255) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ux_users_user_id ON users (user_id);
```

### Shard Key Choice

`user_id` is used as the shard key because:

* It is immutable
* Present in all access paths
* Provides stable distribution
* Is application-controlled

---

## Scaling Characteristics

### Vertical Scaling

Each shard can be independently resized (CPU, RAM, IOPS).

### Horizontal Read Scaling

* Add replicas to hot shards
* No application changes required

### Horizontal Write Scaling

* Requires **adding shards**
* Requires **offline or controlled data migration**

---

## Failure Scenarios

### Replica Failure

* Reads fall back to primary
* Writes unaffected
* Replica can be rebuilt via base backup

### Primary Failure

* Writes unavailable until failover
* Reads may continue from replicas
* Promotion requires external orchestration (manual or tool-based)

> This architecture does **not** include automatic leader election.

---

## Observability & Monitoring

Recommended metrics per shard:

* Replication lag
* Active connections
* Query throughput
* Error rate
* Disk utilization

Global metrics:

* Shard distribution
* Cross-shard queries
* Read/write ratio
* P99 latency

---

## Docker Compose Environment

The provided `docker-compose.yml` is **for local development and testing only**.

It demonstrates:

* One primary per shard
* One replica per shard
* Physical streaming replication

⚠️ **Not suitable for production** due to:

* Static credentials
* No automatic failover
* No persistent backup strategy
* No network isolation

---

## Summary

This architecture:

* Scales writes via **application-level sharding**
* Scales reads via **PostgreSQL replication**
* Offers **predictable consistency trade-offs**
* Keeps shard ownership explicit and simple

It is appropriate for systems where:

* Access patterns are shard-key-centric
* Cross-shard transactions are rare
* Operational simplicity is prioritized over elastic scaling

---

**Status:** Production-capable with proper operational tooling (monitoring, backups, failover automation)

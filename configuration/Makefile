.PHONY: help setup start stop restart down clean logs \
        test test-verbose test-sharding test-repository \
        demo verify-replication status init

ROOT := $(shell cd .. && pwd)

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

setup:
	cd $(ROOT) && go mod download
	cd $(ROOT) && go mod tidy

start: ## Start all database containers
	docker-compose up -d
	@echo "Waiting for databases to be ready..."
	@sleep 10
	@echo "Databases started! Run 'make test' to verify."

stop: ## Stop all database containers
	docker-compose stop

restart: ## Restart all database containers
	docker-compose restart

down: ## Stop and remove all containers
	docker-compose down

clean: ## Stop containers and remove volumes (deletes all data)
	docker-compose down -v

logs: ## Show logs from all containers
	docker-compose logs -f


test:
	cd $(ROOT) && go test ./... -count=1


test-verbose:
	cd $(ROOT) && go test ./... -v -count=1

test-sharding:
	cd $(ROOT) && go test ./sharding -v -count=1

test-repository:
	cd $(ROOT) && go test ./repository -v -count=1

demo:
	cd $(ROOT) && go run main.go

verify-replication:
	@docker exec shard0-primary psql -U postgres -d shard0 \
	  -c "SELECT pid, client_addr, state, sync_state FROM pg_stat_replication;" || true

status: ## Check status of all containers
	docker-compose ps

init: setup start
	@echo "Waiting for DB healthchecks..."
	@sleep 15

#!/bin/bash
set -e

echo "=== Replica Setup Script ==="
echo "This script is executed, but the actual replication setup"
echo "is done by pg_basebackup command in docker-compose.yml"
echo "Replica will automatically sync with its primary server"
echo "============================="
#!/bin/sh
# init-minio.sh — Creates default S3 buckets on MinIO startup.
set -e

# Wait for MinIO to be ready
sleep 2

# Configure the MinIO client alias
mc alias set local http://minio:9000 minioadmin minioadmin

# Create buckets
mc mb local/recordings --ignore-existing
mc mb local/audio-temp --ignore-existing
mc mb local/bot-logs --ignore-existing

# Set public download policy on recordings (for webhook URLs)
mc anonymous set download local/recordings

echo "✅ MinIO buckets initialized successfully"
mc ls local/

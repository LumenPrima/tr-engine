#!/bin/sh
set -e

# Smart entrypoint for tr-engine Docker image
# Supports both all-in-one mode (embedded PostgreSQL) and standalone mode (external services)

# Check if standalone mode is requested
if [ "$TR_ENGINE_STANDALONE" = "true" ] || [ "$TR_ENGINE_STANDALONE" = "1" ]; then
    echo "Starting tr-engine in standalone mode (external PostgreSQL/MQTT)"
    exec /app/tr-engine "$@"
fi

# All-in-one mode: Initialize PostgreSQL if needed, then start via supervisord

# Initialize PostgreSQL data directory if empty
if [ -z "$(ls -A $PGDATA 2>/dev/null)" ]; then
    echo "Initializing PostgreSQL database..."
    su postgres -c "initdb -D $PGDATA"

    # Configure PostgreSQL
    echo "host all all 0.0.0.0/0 md5" >> $PGDATA/pg_hba.conf
    echo "listen_addresses='*'" >> $PGDATA/postgresql.conf

    # Enable TimescaleDB if available
    if [ -f /usr/lib/postgresql15/timescaledb.so ]; then
        echo "shared_preload_libraries = 'timescaledb'" >> $PGDATA/postgresql.conf
    fi

    # Start PostgreSQL temporarily
    su postgres -c "pg_ctl -D $PGDATA -w start"

    # Create user and database
    su postgres -c "psql -c \"CREATE USER $POSTGRES_USER WITH PASSWORD '$POSTGRES_PASSWORD';\""
    su postgres -c "psql -c \"CREATE DATABASE $POSTGRES_DB OWNER $POSTGRES_USER;\""
    su postgres -c "psql -d $POSTGRES_DB -c \"CREATE EXTENSION IF NOT EXISTS timescaledb;\"" || true

    # Stop PostgreSQL (supervisor will start it)
    su postgres -c "pg_ctl -D $PGDATA -w stop"

    echo "PostgreSQL initialization complete."
fi

echo "Starting tr-engine in all-in-one mode (embedded PostgreSQL)"
exec /usr/bin/supervisord -c /etc/supervisord.conf

#!/bin/bash
set -e

# Initialize PostgreSQL data directory if empty
if [ -z "$(ls -A $PGDATA)" ]; then
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

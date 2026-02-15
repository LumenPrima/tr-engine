-- Run this as the postgres admin user:
--   psql -h oracle -U postgres -f setup-db.sql

-- Create the database
CREATE DATABASE trengine;

-- Create a dedicated application user
CREATE USER trengine WITH PASSWORD 'trengine';

-- Connect to the new database to set permissions
\c trengine

-- Grant privileges
GRANT CONNECT ON DATABASE trengine TO trengine;
GRANT USAGE ON SCHEMA public TO trengine;
GRANT CREATE ON SCHEMA public TO trengine;

-- Grant on all current and future tables
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO trengine;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO trengine;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT EXECUTE ON FUNCTIONS TO trengine;

-- Now run schema.sql as the postgres admin (so trengine user gets access via defaults above):
--   psql -h oracle -U postgres -d trengine -f schema.sql

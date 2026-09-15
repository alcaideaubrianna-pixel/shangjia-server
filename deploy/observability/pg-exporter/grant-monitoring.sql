-- Run as the database owner after installing application schema changes.
-- The exporter is read-only; these grants let custom probes follow active
-- channel/profile relations without granting any mutation privileges.
GRANT USAGE ON SCHEMA public TO dbuser_monitor;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO dbuser_monitor;

-- Keep monitoring functional for tables created by future migrations.
ALTER DEFAULT PRIVILEGES IN SCHEMA public
GRANT SELECT ON TABLES TO dbuser_monitor;

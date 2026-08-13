SELECT pg_replication_origin_drop(roname)
FROM pg_replication_origin
WHERE roname = 'pgcopydb';

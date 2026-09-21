-- Undo the entire migration. Never run in production unless you mean it.
DROP TABLE IF EXISTS auth.users;
DROP SCHEMA IF EXISTS auth;
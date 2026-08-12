-- Local-dev passwords for the roles bootstrap-rls-roles.sql creates with
-- REPLACE_ME placeholders. Matches the DSNs in .env.local. Docker Compose only —
-- remote environments get their passwords from Doppler.

ALTER ROLE comanda_privileged WITH PASSWORD 'comanda_privileged';
ALTER ROLE comanda_app        WITH PASSWORD 'comanda_app';

#!/bin/sh
set -eu
password="$(cat /run/connector-engine/pg-app-password)"
psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres <<SQL
CREATE ROLE cn LOGIN PASSWORD '${password}' NOSUPERUSER NOBYPASSRLS;
CREATE DATABASE connector_engine OWNER cn;
SQL

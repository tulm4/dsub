-- Based on: docs/data-model.md §3.0 (Database and Schema Setup)
-- 3GPP: TS 29.505 — UDM database schema initialization

CREATE SCHEMA IF NOT EXISTS udm;

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Based on: docs/data-model.md §5.4 (Cross-Region Data Placement Policies)
-- 3GPP: TS 29.505 — Multi-region data placement for subscriber data
--
-- Tablespace configuration for geo-distributed YugabyteDB deployments.
-- These tablespaces use YugabyteDB's replica_placement to control where
-- tablet replicas are placed across cloud regions.
--
-- NOTE: Tablespace creation requires a multi-node YugabyteDB cluster with
-- placement information configured on each tserver. In single-node or
-- development environments, this migration should be skipped.
-- The migration runner's ApplyNoTx method must be used because
-- CREATE TABLESPACE cannot execute inside a transaction block.
--
-- Idempotency: uses DO/EXCEPTION blocks to handle pre-existing tablespaces.

-- Global tablespace: replicas spread across 3 regions (RF=3)
-- Used by core subscriber tables for global availability
DO $$
BEGIN
    EXECUTE 'CREATE TABLESPACE ts_global WITH (
        replica_placement = ''{"num_replicas": 3, "placement_blocks": [
            {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
            {"cloud": "aws", "region": "us-west-2", "zone": "us-west-2a", "min_num_replicas": 1},
            {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1a", "min_num_replicas": 1}
        ]}'')';
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;

-- US-local tablespace: all replicas within US regions (RF=3)
-- Used for latency-sensitive registration data when subscriber's home
-- PLMN is a US operator (MCC 310-316)
DO $$
BEGIN
    EXECUTE 'CREATE TABLESPACE ts_us_local WITH (
        replica_placement = ''{"num_replicas": 3, "placement_blocks": [
            {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
            {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1b", "min_num_replicas": 1},
            {"cloud": "aws", "region": "us-west-2", "zone": "us-west-2a", "min_num_replicas": 1}
        ]}'')';
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;

-- EU-local tablespace: all replicas within EU regions (RF=3)
-- Used for GDPR-compliant data placement when subscriber's home
-- PLMN is an EU operator
DO $$
BEGIN
    EXECUTE 'CREATE TABLESPACE ts_eu_local WITH (
        replica_placement = ''{"num_replicas": 3, "placement_blocks": [
            {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1a", "min_num_replicas": 1},
            {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1b", "min_num_replicas": 1},
            {"cloud": "aws", "region": "eu-central-1", "zone": "eu-central-1a", "min_num_replicas": 1}
        ]}'')';
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;

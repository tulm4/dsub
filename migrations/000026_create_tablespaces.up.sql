-- Based on: docs/data-model.md §5.4 (Cross-Region Data Placement Policies)
-- 3GPP: TS 29.505 — Multi-region data placement for subscriber data
--
-- Tablespace configuration for geo-distributed YugabyteDB deployments.
-- These tablespaces use YugabyteDB's replica_placement to control where
-- tablet replicas are placed across cloud regions.
--
-- NOTE: Tablespace creation requires a multi-node YugabyteDB cluster with
-- placement information configured on each tserver. In single-node or
-- development environments, these statements are safely skipped via
-- the migration runner's error handling (tablespace creation is idempotent
-- when re-applied).

-- Global tablespace: replicas spread across 3 regions (RF=3)
-- Used by core subscriber tables for global availability
CREATE TABLESPACE IF NOT EXISTS ts_global WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-west-2", "zone": "us-west-2a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1a", "min_num_replicas": 1}
    ]}'
);

-- US-local tablespace: all replicas within US regions (RF=3)
-- Used for latency-sensitive registration data when subscriber's home
-- PLMN is a US operator (MCC 310-316)
CREATE TABLESPACE IF NOT EXISTS ts_us_local WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1b", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-west-2", "zone": "us-west-2a", "min_num_replicas": 1}
    ]}'
);

-- EU-local tablespace: all replicas within EU regions (RF=3)
-- Used for GDPR-compliant data placement when subscriber's home
-- PLMN is an EU operator
CREATE TABLESPACE IF NOT EXISTS ts_eu_local WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1b", "min_num_replicas": 1},
        {"cloud": "aws", "region": "eu-central-1", "zone": "eu-central-1a", "min_num_replicas": 1}
    ]}'
);

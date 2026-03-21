-- Rollback: docs/data-model.md §5.4 (Cross-Region Data Placement Policies)
DROP TABLESPACE IF EXISTS ts_eu_local;
DROP TABLESPACE IF EXISTS ts_us_local;
DROP TABLESPACE IF EXISTS ts_global;

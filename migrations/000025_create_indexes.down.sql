-- Rollback: docs/data-model.md §4 (Indexing Strategy)

-- §4.4 JSONB GIN Indexes
DROP INDEX IF EXISTS udm.idx_sm_dnn_configs_gin;
DROP INDEX IF EXISTS udm.idx_ee_monitoring_gin;
DROP INDEX IF EXISTS udm.idx_am_nssai_gin;

-- §4.3 Covering Indexes
DROP INDEX IF EXISTS udm.idx_sm_data_covering;
DROP INDEX IF EXISTS udm.idx_amf_reg_covering;
DROP INDEX IF EXISTS udm.idx_auth_covering;
DROP INDEX IF EXISTS udm.idx_am_data_covering;

-- §4.2 Secondary Indexes
DROP INDEX IF EXISTS udm.idx_audit_time;
DROP INDEX IF EXISTS udm.idx_audit_supi_time;
DROP INDEX IF EXISTS udm.idx_opdata_supi;
DROP INDEX IF EXISTS udm.idx_smf_reg_dnn_nssai;
DROP INDEX IF EXISTS udm.idx_smf_reg_instance;
DROP INDEX IF EXISTS udm.idx_amf_reg_instance;
DROP INDEX IF EXISTS udm.idx_sdm_subs_supi;
DROP INDEX IF EXISTS udm.idx_ee_subs_supi;
DROP INDEX IF EXISTS udm.idx_ee_subs_gpsi;
DROP INDEX IF EXISTS udm.idx_ee_subs_group;
DROP INDEX IF EXISTS udm.idx_subscribers_gpsi;

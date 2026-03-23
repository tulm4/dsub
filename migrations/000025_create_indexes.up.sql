-- Based on: docs/data-model.md §4 (Indexing Strategy)
-- Secondary indexes, covering indexes, and JSONB GIN indexes

-- §4.2 Secondary Indexes for Lookup Patterns

-- GPSI (MSISDN) lookups: resolve MSISDN -> SUPI
CREATE INDEX idx_subscribers_gpsi
    ON udm.subscribers (gpsi)
    WHERE gpsi IS NOT NULL
    SPLIT INTO 2 TABLETS;

-- Group-based queries for EE subscriptions
CREATE INDEX idx_ee_subs_group
    ON udm.ee_subscriptions (ue_group_id)
    WHERE ue_group_id IS NOT NULL
    SPLIT INTO 2 TABLETS;

-- GPSI-based EE subscription lookups
CREATE INDEX idx_ee_subs_gpsi
    ON udm.ee_subscriptions (gpsi)
    WHERE gpsi IS NOT NULL
    SPLIT INTO 2 TABLETS;

-- EE subscriptions by SUPI
CREATE INDEX idx_ee_subs_supi
    ON udm.ee_subscriptions (supi)
    WHERE supi IS NOT NULL
    SPLIT INTO 2 TABLETS;

-- SDM subscriptions by SUPI
CREATE INDEX idx_sdm_subs_supi
    ON udm.sdm_subscriptions (supi)
    SPLIT INTO 2 TABLETS;

-- AMF instance lookups (find all subscribers registered to a specific AMF)
CREATE INDEX idx_amf_reg_instance
    ON udm.amf_registrations (amf_instance_id)
    SPLIT INTO 2 TABLETS;

-- SMF instance lookups
CREATE INDEX idx_smf_reg_instance
    ON udm.smf_registrations (smf_instance_id)
    SPLIT INTO 2 TABLETS;

-- SMF registration by DNN and slice (slice-aware queries)
CREATE INDEX idx_smf_reg_dnn_nssai
    ON udm.smf_registrations (dnn, single_nssai)
    SPLIT INTO 2 TABLETS;

-- Operator-specific data by type
CREATE INDEX idx_opdata_supi
    ON udm.operator_specific_data (supi)
    SPLIT INTO 2 TABLETS;

-- Audit log: time-based queries and per-subscriber audit
CREATE INDEX idx_audit_supi_time
    ON udm.audit_log (supi, created_at DESC)
    SPLIT INTO 2 TABLETS;

CREATE INDEX idx_audit_time
    ON udm.audit_log (created_at DESC)
    SPLIT INTO 2 TABLETS;

-- §4.3 Covering Indexes for Hot Paths

-- SDM hot path: AMF retrieves AM data for a subscriber.
CREATE INDEX idx_am_data_covering
    ON udm.access_mobility_subscription (supi, serving_plmn_id)
    INCLUDE (nssai, gpsis, subscribed_ue_ambr, rat_restrictions, rfsp_index)
    SPLIT INTO 2 TABLETS;

-- Authentication hot path: UEAU reads auth method and key references.
CREATE INDEX idx_auth_covering
    ON udm.authentication_data (supi)
    INCLUDE (auth_method, algorithm_id, sqn, amf_value)
    SPLIT INTO 2 TABLETS;

-- AMF registration hot path: SDM checks if subscriber is registered.
CREATE INDEX idx_amf_reg_covering
    ON udm.amf_registrations (supi)
    INCLUDE (amf_instance_id, rat_type, access_type, dereg_callback_uri)
    SPLIT INTO 2 TABLETS;

-- SM data hot path: SMF retrieves DNN configs for a subscriber/PLMN/slice.
CREATE INDEX idx_sm_data_covering
    ON udm.session_management_subscription (supi, serving_plmn_id)
    INCLUDE (nssai_sst, nssai_sd, single_nssai, dnn_configurations)
    SPLIT INTO 2 TABLETS;

-- §4.4 JSONB Indexes for Nested Queries

-- GIN index on NSSAI for slice-based queries
CREATE INDEX idx_am_nssai_gin
    ON udm.access_mobility_subscription USING GIN (nssai);

-- GIN index on monitoring configurations for event type filtering
CREATE INDEX idx_ee_monitoring_gin
    ON udm.ee_subscriptions USING GIN (monitoring_configurations);

-- GIN index on DNN configurations
CREATE INDEX idx_sm_dnn_configs_gin
    ON udm.session_management_subscription USING GIN (dnn_configurations);

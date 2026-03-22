-- Based on: docs/data-model.md §3.5 (Per-DNN/S-NSSAI SM Data)
-- 3GPP: TS 29.505 — SessionManagementSubscriptionData

CREATE TABLE udm.session_management_subscription (
    supi                    TEXT        NOT NULL,
    serving_plmn_id         TEXT        NOT NULL DEFAULT '00000',
    nssai_sst               INTEGER     NOT NULL,
    nssai_sd                TEXT        NOT NULL DEFAULT '',
    single_nssai            JSONB       NOT NULL,
    dnn_configurations      JSONB       NOT NULL DEFAULT '{}'::JSONB,
    internal_group_ids      TEXT[],
    shared_data_ids         TEXT[],
    shared_sm_subs_data_ids TEXT[],
    odb_packet_services     TEXT,
    trace_data              JSONB,
    shared_trace_data_id    TEXT,
    expected_ue_behaviours  JSONB,
    suggested_packet_num_dl JSONB,
    three_gpp_charging_characteristics TEXT,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id, nssai_sst, nssai_sd),
    CONSTRAINT fk_sm_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

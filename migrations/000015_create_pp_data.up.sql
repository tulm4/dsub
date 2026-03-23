-- Based on: docs/data-model.md §3.14 (Parameter Provisioning Data)
-- 3GPP: TS 29.503 Nudm_PP — PpData

CREATE TABLE udm.pp_data (
    supi                            TEXT        NOT NULL,
    communication_characteristics   JSONB,
    supported_features              TEXT,
    expected_ue_behaviour           JSONB,
    ec_restriction                  JSONB,
    acs_info                        JSONB,
    sor_info                        JSONB,
    five_mbs_authorization_info     JSONB,
    steering_container              JSONB,
    pp_dl_packet_count              INTEGER,
    pp_dl_packet_count_ext          JSONB,
    pp_maximum_response_time        INTEGER,
    pp_maximum_latency              INTEGER,
    version                         BIGINT      NOT NULL DEFAULT 1,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_pp_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

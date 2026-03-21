-- Based on: docs/data-model.md §3.2 (Auth Credentials)
-- 3GPP: TS 29.505 — Authentication Subscription Data
-- 3GPP: TS 33.501 §6.1 — Authentication credential storage

CREATE TABLE udm.authentication_data (
    supi                        TEXT        NOT NULL,
    auth_method                 TEXT        NOT NULL
                                            CHECK (auth_method IN (
                                                '5G_AKA', 'EAP_AKA_PRIME', 'EAP_TLS',
                                                'EAP_TTLS', 'NONE'
                                            )),
    k_key                       BYTEA,
    opc_key                     BYTEA,
    topc_key                    BYTEA,
    sqn                         TEXT        DEFAULT '000000000000',
    sqn_scheme                  TEXT        DEFAULT 'NON_TIME_BASED'
                                            CHECK (sqn_scheme IN (
                                                'GENERAL', 'NON_TIME_BASED', 'TIME_BASED'
                                            )),
    sqn_last_indexes            JSONB       DEFAULT '{}'::JSONB,
    sqn_ind_length              INTEGER     DEFAULT 5,
    amf_value                   TEXT        DEFAULT '8000',
    algorithm_id                TEXT,
    protection_parameter_id     TEXT,
    vector_generation_in_hss    BOOLEAN     NOT NULL DEFAULT FALSE,
    hss_group_id                TEXT,
    n5gc_auth_method            TEXT,
    rg_authentication_ind       BOOLEAN     NOT NULL DEFAULT FALSE,
    akma_allowed                BOOLEAN     NOT NULL DEFAULT FALSE,
    routing_id                  TEXT,
    nswo_allowed                BOOLEAN     NOT NULL DEFAULT FALSE,
    version                     BIGINT      NOT NULL DEFAULT 1,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_auth_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

-- Based on: docs/service-decomposition.md §2.5 (udm-pp — 5G VN Group Management)
-- 3GPP: TS 29.503 Nudm_PP — 5GVnGroupConfiguration

CREATE TABLE udm.vn_groups (
    ext_group_id            TEXT        NOT NULL,
    dnn                     TEXT,
    s_nssai                 JSONB,
    pdu_session_types       TEXT[],
    app_descriptors         JSONB,
    secondary_auth          BOOLEAN     DEFAULT FALSE,
    dn_aaa_address          JSONB,
    dn_aaa_fqdn             TEXT,
    members                 JSONB       NOT NULL DEFAULT '[]'::JSONB,
    reference_id            TEXT,
    af_instance_id          TEXT,
    internal_group_identifier TEXT,
    mtc_provider_information JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (ext_group_id)
) SPLIT INTO 32 TABLETS;

-- Based on: docs/data-model.md (Phase 6 — Low-Traffic Services)
-- Based on: docs/service-decomposition.md §2.7 (udm-ssau)
-- 3GPP: TS 29.503 Nudm_SSAU — Service-Specific Authorization

CREATE TABLE udm.ssau_authorizations (
    auth_id                 UUID        NOT NULL DEFAULT gen_random_uuid(),
    supi                    TEXT,
    gpsi                    TEXT,
    ue_group_id             TEXT,
    service_type            TEXT        NOT NULL,
    snssai                  JSONB,
    dnn                     TEXT,
    authorization_data      JSONB       NOT NULL DEFAULT '{}'::JSONB,
    auth_callback_uri       TEXT,
    af_id                   TEXT,
    nef_id                  TEXT,
    validity_time           TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version                 BIGINT      NOT NULL DEFAULT 1,

    PRIMARY KEY (auth_id)
) SPLIT INTO 2 TABLETS;

CREATE INDEX idx_ssau_authorizations_supi ON udm.ssau_authorizations (supi)
    WHERE supi IS NOT NULL;
CREATE INDEX idx_ssau_authorizations_gpsi ON udm.ssau_authorizations (gpsi)
    WHERE gpsi IS NOT NULL;
CREATE INDEX idx_ssau_authorizations_ue_group_id ON udm.ssau_authorizations (ue_group_id)
    WHERE ue_group_id IS NOT NULL;

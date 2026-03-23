-- Based on: docs/data-model.md §3.15 (PP Profile Data)
-- 3GPP: TS 29.503 Nudm_PP — PpProfileData

CREATE TABLE udm.pp_profile_data (
    supi                            TEXT        NOT NULL,
    allowed_mtc_providers           JSONB,
    supported_features              TEXT,
    version                         BIGINT      NOT NULL DEFAULT 1,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_pp_profile_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

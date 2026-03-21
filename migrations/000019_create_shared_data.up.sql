-- Based on: docs/data-model.md §3.18 (Shared Subscription Profiles)
-- 3GPP: TS 29.505 — SharedData

CREATE TABLE udm.shared_data (
    shared_data_id      TEXT        NOT NULL,
    shared_data_type    TEXT        NOT NULL,
    data                JSONB       NOT NULL,
    description         TEXT,
    supported_features  TEXT,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (shared_data_id)
) SPLIT INTO 16 TABLETS;

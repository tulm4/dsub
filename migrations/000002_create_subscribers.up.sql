-- Based on: docs/data-model.md §3.1 (Core Subscriber Table)
-- 3GPP: TS 29.505 — Subscription Data
-- 3GPP: TS 23.003 — SUPI as root key

CREATE TABLE udm.subscribers (
    supi                TEXT        NOT NULL,
    gpsi                TEXT,
    supi_type           TEXT        NOT NULL DEFAULT 'imsi'
                                   CHECK (supi_type IN ('imsi', 'nai', 'gci', 'gli')),
    gpsi_type           TEXT        CHECK (gpsi_type IN ('msisdn', 'external_id')),
    group_ids           TEXT[],
    identity_data       JSONB,
    odb_data            JSONB,
    roaming_allowed     BOOLEAN     NOT NULL DEFAULT TRUE,
    provisioning_status TEXT        NOT NULL DEFAULT 'active'
                                   CHECK (provisioning_status IN ('active', 'suspended', 'pending_deletion')),
    shared_data_ids     TEXT[],
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi)
) SPLIT INTO 128 TABLETS;

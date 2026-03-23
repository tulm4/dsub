-- Based on: docs/data-model.md §3.16 (Slice-Specific Subscription Data)
-- 3GPP: TS 29.505 — NssaiData, network slice subscription

CREATE TABLE udm.network_slice_data (
    supi                    TEXT        NOT NULL,
    nssai                   JSONB       NOT NULL DEFAULT '[]'::JSONB,
    default_single_nssais   JSONB       NOT NULL DEFAULT '[]'::JSONB,
    single_nssais           JSONB       NOT NULL DEFAULT '[]'::JSONB,
    mapping_of_nssai        JSONB,
    suppressed_nssai        JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_nsd_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

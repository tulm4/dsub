-- Based on: docs/data-model.md §3.21 (IP-SM-GW Context)
-- 3GPP: TS 29.503 Nudm_UECM — IpSmGwRegistration

CREATE TABLE udm.ip_sm_gw_registrations (
    supi                TEXT        NOT NULL,
    ip_sm_gw_map_address TEXT,
    unri_indicator      BOOLEAN     NOT NULL DEFAULT FALSE,
    reset_ids           TEXT[],
    context_info        JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_ipsmgw_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

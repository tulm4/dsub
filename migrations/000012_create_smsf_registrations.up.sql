-- Based on: docs/data-model.md §3.11 (SMSF Registrations)
-- 3GPP: TS 29.503 Nudm_UECM — SmsfRegistration

CREATE TABLE udm.smsf_registrations (
    supi                    TEXT        NOT NULL,
    access_type             TEXT        NOT NULL
                                       CHECK (access_type IN (
                                           '3GPP_ACCESS', 'NON_3GPP_ACCESS'
                                       )),
    smsf_instance_id        UUID        NOT NULL,
    smsf_set_id             TEXT,
    smsf_service_instance_id TEXT,
    plmn_id                 JSONB       NOT NULL,
    smsf_map_address        TEXT,
    ue_reachable            BOOLEAN     NOT NULL DEFAULT TRUE,
    registration_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, access_type),
    CONSTRAINT fk_smsf_reg_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

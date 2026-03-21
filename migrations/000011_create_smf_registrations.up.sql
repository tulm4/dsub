-- Based on: docs/data-model.md §3.10 (SMF PDU Session Registrations)
-- 3GPP: TS 29.503 Nudm_UECM — SmfRegistration

CREATE TABLE udm.smf_registrations (
    supi                    TEXT        NOT NULL,
    pdu_session_id          INTEGER     NOT NULL CHECK (pdu_session_id BETWEEN 1 AND 255),
    smf_instance_id         UUID        NOT NULL,
    smf_set_id              TEXT,
    smf_service_instance_id TEXT,
    dnn                     TEXT        NOT NULL,
    single_nssai            JSONB       NOT NULL,
    plmn_id                 JSONB       NOT NULL,
    emergency_services      BOOLEAN     NOT NULL DEFAULT FALSE,
    pcscf_restoration_callback_uri TEXT,
    pdu_session_type        TEXT        CHECK (pdu_session_type IN (
                                           'IPV4', 'IPV6', 'IPV4V6',
                                           'UNSTRUCTURED', 'ETHERNET'
                                       )),
    pgw_fqdn                TEXT,
    registration_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registration_reason     TEXT        CHECK (registration_reason IN (
                                           'INITIAL_REGISTRATION',
                                           'CHANGE_REGISTRATION'
                                       )),
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, pdu_session_id),
    CONSTRAINT fk_smf_reg_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

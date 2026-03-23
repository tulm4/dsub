-- Based on: docs/data-model.md §3.9 (AMF Context Registrations)
-- 3GPP: TS 29.503 Nudm_UECM — Amf3GppAccessRegistration / AmfNon3GppAccessRegistration

CREATE TABLE udm.amf_registrations (
    supi                    TEXT        NOT NULL,
    access_type             TEXT        NOT NULL
                                       CHECK (access_type IN (
                                           '3GPP_ACCESS', 'NON_3GPP_ACCESS'
                                       )),
    amf_instance_id         UUID        NOT NULL,
    dereg_callback_uri      TEXT        NOT NULL,
    amf_service_name_dereg  TEXT,
    pcscf_restoration_callback_uri TEXT,
    amf_service_name_pcscf_rest TEXT,
    guami                   JSONB       NOT NULL,
    rat_type                TEXT        NOT NULL,
    ue_reachable            BOOLEAN     NOT NULL DEFAULT TRUE,
    initial_registration_ind BOOLEAN    NOT NULL DEFAULT FALSE,
    emergency_registration_ind BOOLEAN  NOT NULL DEFAULT FALSE,
    ims_vo_ps               TEXT        CHECK (ims_vo_ps IN (
                                           'HOMOGENEOUS_SUPPORT',
                                           'HOMOGENEOUS_NON_SUPPORT',
                                           'NON_HOMOGENEOUS_OR_UNKNOWN'
                                       )),
    plmn_id                 JSONB,
    backup_amf_info         JSONB,
    dr_flag                 BOOLEAN     NOT NULL DEFAULT FALSE,
    supi_pei_available      BOOLEAN     NOT NULL DEFAULT FALSE,
    ue_srvcc_capability     BOOLEAN     NOT NULL DEFAULT FALSE,
    registration_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    no_ee_subscription_ind  BOOLEAN     NOT NULL DEFAULT FALSE,
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, access_type),
    CONSTRAINT fk_amf_reg_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 4 TABLETS;

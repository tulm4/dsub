-- Based on: docs/data-model.md §3.8 (SMS Management Subscription Data)
-- 3GPP: TS 29.505 — SmsManagementSubscriptionData

CREATE TABLE udm.sms_management_data (
    supi                TEXT        NOT NULL,
    serving_plmn_id     TEXT        NOT NULL DEFAULT '00000',
    mt_sms_subscribed   BOOLEAN     NOT NULL DEFAULT TRUE,
    mt_sms_barring_all  BOOLEAN     NOT NULL DEFAULT FALSE,
    mt_sms_barring_roaming BOOLEAN  NOT NULL DEFAULT FALSE,
    mo_sms_subscribed   BOOLEAN     NOT NULL DEFAULT TRUE,
    mo_sms_barring_all  BOOLEAN     NOT NULL DEFAULT FALSE,
    mo_sms_barring_roaming BOOLEAN  NOT NULL DEFAULT FALSE,
    shared_data_ids     TEXT[],
    trace_data          JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_sms_mng_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

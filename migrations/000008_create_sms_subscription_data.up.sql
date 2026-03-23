-- Based on: docs/data-model.md §3.7 (SMS Subscription Data)
-- 3GPP: TS 29.505 — SmsSubscriptionData

CREATE TABLE udm.sms_subscription_data (
    supi                TEXT        NOT NULL,
    serving_plmn_id     TEXT        NOT NULL DEFAULT '00000',
    sms_subscribed      BOOLEAN     NOT NULL DEFAULT TRUE,
    shared_data_ids     TEXT[],
    sms_data            JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_sms_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

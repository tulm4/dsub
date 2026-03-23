-- Based on: docs/data-model.md §3.12 (Event Exposure Subscriptions)
-- 3GPP: TS 29.503 Nudm_EE — EeSubscription

CREATE TABLE udm.ee_subscriptions (
    subscription_id         TEXT        NOT NULL DEFAULT gen_random_uuid()::TEXT,
    supi                    TEXT,
    gpsi                    TEXT,
    ue_group_id             TEXT,
    callback_reference      TEXT        NOT NULL,
    monitoring_configurations JSONB     NOT NULL DEFAULT '{}'::JSONB,
    reporting_options        JSONB,
    supported_features      TEXT,
    subscription_data_subscriptions JSONB,
    scef_id                 TEXT,
    nf_instance_id          TEXT,
    data_restoration_callback_uri TEXT,
    excluded_unsubscribed_ues BOOLEAN   NOT NULL DEFAULT FALSE,
    immediate_report_data   JSONB,
    expiry_time             TIMESTAMPTZ,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (subscription_id),
    CONSTRAINT fk_ee_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE,
    CONSTRAINT chk_ee_identity
        CHECK (supi IS NOT NULL OR gpsi IS NOT NULL OR ue_group_id IS NOT NULL)
) SPLIT INTO 64 TABLETS;

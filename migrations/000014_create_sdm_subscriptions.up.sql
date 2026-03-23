-- Based on: docs/data-model.md §3.13 (SDM Change Notification Subscriptions)
-- 3GPP: TS 29.503 Nudm_SDM — SdmSubscription

CREATE TABLE udm.sdm_subscriptions (
    subscription_id         TEXT        NOT NULL DEFAULT gen_random_uuid()::TEXT,
    supi                    TEXT        NOT NULL,
    callback_reference      TEXT        NOT NULL,
    monitored_resource_uris TEXT[]      NOT NULL,
    nf_instance_id          UUID        NOT NULL,
    implicit_unsubscribe    BOOLEAN     NOT NULL DEFAULT FALSE,
    supported_features      TEXT,
    expiry_time             TIMESTAMPTZ,
    single_nssai            JSONB,
    dnn                     TEXT,
    plmn_id                 JSONB,
    immediate_report        BOOLEAN     NOT NULL DEFAULT FALSE,
    data_restoration_callback_uri TEXT,
    unique_subscription     BOOLEAN     NOT NULL DEFAULT FALSE,
    report                  JSONB,
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (subscription_id),
    CONSTRAINT fk_sdm_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

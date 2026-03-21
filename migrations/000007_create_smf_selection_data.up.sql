-- Based on: docs/data-model.md §3.6 (SMF Selection Subscription Data)
-- 3GPP: TS 29.505 — SmfSelectionSubscriptionData

CREATE TABLE udm.smf_selection_data (
    supi                    TEXT        NOT NULL,
    serving_plmn_id         TEXT        NOT NULL DEFAULT '00000',
    subscribed_snssai_infos JSONB,
    shared_data_ids         TEXT[],
    shared_snssai_infos     JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_smf_sel_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

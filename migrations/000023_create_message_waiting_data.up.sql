-- Based on: docs/data-model.md §3.22 (Message Waiting Indication)
-- 3GPP: TS 29.503 Nudm_MT — MessageWaitingData

CREATE TABLE udm.message_waiting_data (
    supi                TEXT        NOT NULL,
    mwd_list            JSONB       NOT NULL DEFAULT '[]'::JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_mwd_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

-- Based on: docs/data-model.md §3.19 (UE Update Confirmation Data)
-- 3GPP: TS 29.503 Nudm_SDM — UeUpdateConfirmation

CREATE TABLE udm.ue_update_confirmation (
    supi                TEXT        NOT NULL,
    sor_data            JSONB,
    upu_data            JSONB,
    subscribed_snssais_ack JSONB,
    subscribed_cag_ack  JSONB,
    ue_update_status    TEXT        CHECK (ue_update_status IN (
                                       'NOT_SENT', 'SENT_NOT_ACKED', 'ACKED'
                                   )),
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_uuc_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

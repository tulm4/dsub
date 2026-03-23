-- Based on: docs/data-model.md (Phase 6 — Low-Traffic Services)
-- Based on: docs/service-decomposition.md §2.9 (udm-rsds)
-- 3GPP: TS 29.503 Nudm_RSDS — Report SMS Delivery Status

CREATE TABLE udm.sms_delivery_status (
    delivery_id             UUID        NOT NULL DEFAULT gen_random_uuid(),
    supi                    TEXT,
    gpsi                    TEXT        NOT NULL,
    sms_status_report       JSONB       NOT NULL DEFAULT '{}'::JSONB,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (delivery_id)
) SPLIT INTO 2 TABLETS;

CREATE INDEX idx_sms_delivery_status_gpsi ON udm.sms_delivery_status (gpsi);
CREATE INDEX idx_sms_delivery_status_supi ON udm.sms_delivery_status (supi)
    WHERE supi IS NOT NULL;

-- Based on: docs/data-model.md §3.20 (Subscriber Trace Configuration)
-- 3GPP: TS 29.505 — TraceData

CREATE TABLE udm.trace_data (
    supi                TEXT        NOT NULL,
    serving_plmn_id     TEXT        NOT NULL DEFAULT '00000',
    trace_ref           TEXT,
    trace_depth         TEXT,
    ne_type_list        TEXT,
    event_list          TEXT,
    collection_entity_ipv4 INET,
    collection_entity_ipv6 INET,
    interface_list      TEXT,
    shared_trace_data_id TEXT,
    trace_data_json     JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_trace_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

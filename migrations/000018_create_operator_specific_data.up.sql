-- Based on: docs/data-model.md §3.17 (Extensible Operator Data)
-- 3GPP: TS 29.505 — OperatorSpecificData

CREATE TABLE udm.operator_specific_data (
    id                  BIGINT      GENERATED ALWAYS AS IDENTITY,
    supi                TEXT        NOT NULL,
    data_type           TEXT        NOT NULL,
    data_type_definition TEXT,
    data_value          JSONB       NOT NULL,
    supported_features  TEXT,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id),
    CONSTRAINT fk_opdata_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE,
    CONSTRAINT uq_opdata_supi_type UNIQUE (supi, data_type)
) SPLIT INTO 2 TABLETS;

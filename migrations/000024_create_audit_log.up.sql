-- Based on: docs/data-model.md §3.23 (Change Audit Trail)
-- Tracks all data mutations for compliance and debugging

CREATE TABLE udm.audit_log (
    id                  BIGINT      GENERATED ALWAYS AS IDENTITY,
    supi                TEXT        NOT NULL,
    table_name          TEXT        NOT NULL,
    operation           TEXT        NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    old_data            JSONB,
    new_data            JSONB,
    changed_by          TEXT,
    nf_instance_id      UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (id ASC)
) SPLIT INTO 2 TABLETS;

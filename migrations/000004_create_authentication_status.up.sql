-- Based on: docs/data-model.md §3.3 (Auth Event Log)
-- 3GPP: TS 29.505 — AuthenticationStatus data type

CREATE TABLE udm.authentication_status (
    supi                TEXT        NOT NULL,
    serving_network_name TEXT       NOT NULL,
    auth_type           TEXT        NOT NULL
                                   CHECK (auth_type IN (
                                       '5G_AKA', 'EAP_AKA_PRIME', 'EAP_TLS'
                                   )),
    success             BOOLEAN     NOT NULL,
    time_stamp          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    auth_removal_ind    BOOLEAN     NOT NULL DEFAULT FALSE,
    nf_instance_id      UUID,
    reset_ids           TEXT[],

    PRIMARY KEY (supi, serving_network_name),
    CONSTRAINT fk_auth_status_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 4 TABLETS;

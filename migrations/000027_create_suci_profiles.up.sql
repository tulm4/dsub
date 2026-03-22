-- Phase 4: SUCI de-concealment home network key profiles
-- Based on: docs/service-decomposition.md §2.10 (udm-ueid)
-- Based on: docs/security.md §4 (Subscriber Identity Protection)
-- 3GPP: TS 33.501 §6.12 — SUCI de-concealment, ECIES Profile A and B

CREATE TABLE udm.suci_profiles (
    hn_key_id       INTEGER         NOT NULL CHECK (hn_key_id >= 0 AND hn_key_id <= 255),
    profile_type    TEXT            NOT NULL CHECK (profile_type IN ('A', 'B')),
    public_key      BYTEA           NOT NULL,
    private_key     BYTEA           NOT NULL,
    is_active       BOOLEAN         NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    PRIMARY KEY (hn_key_id)
);

-- Based on: docs/service-decomposition.md §2.5 (udm-pp — 5G VN Group Members)
-- 3GPP: TS 29.503 Nudm_PP — 5GVnGroupConfiguration members

CREATE TABLE udm.vn_group_members (
    ext_group_id    TEXT        NOT NULL,
    gpsi            TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (ext_group_id, gpsi),
    CONSTRAINT fk_vn_group_members_group
        FOREIGN KEY (ext_group_id) REFERENCES udm.vn_groups(ext_group_id) ON DELETE CASCADE
) SPLIT INTO 2 TABLETS;

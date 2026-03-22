-- Based on: docs/service-decomposition.md §2.5 (udm-pp — MBS Group Membership)
-- 3GPP: TS 29.503 Nudm_PP — MulticastMbsGroupMemb

CREATE TABLE udm.mbs_group_membership (
    ext_group_id            TEXT        NOT NULL,
    multicast_group_memb    JSONB       NOT NULL DEFAULT '[]'::JSONB,
    af_instance_id          TEXT,
    internal_group_identifier TEXT,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (ext_group_id)
) SPLIT INTO 16 TABLETS;

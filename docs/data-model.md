# 5G UDM Data Model & YugabyteDB Schema Design

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |

---

## Table of Contents

1. [Data Model Overview](#1-data-model-overview)
2. [Entity-Relationship Model](#2-entity-relationship-model)
3. [YugabyteDB Schema Design](#3-yugabytedb-schema-design)
4. [Indexing Strategy](#4-indexing-strategy)
5. [Data Partitioning and Sharding Strategy](#5-data-partitioning-and-sharding-strategy)
6. [YugabyteDB-Specific Optimizations](#6-yugabytedb-specific-optimizations)
7. [Transaction Strategy](#7-transaction-strategy)
8. [Consistency Model](#8-consistency-model)
9. [Data Migration and Versioning](#9-data-migration-and-versioning)
10. [Storage Estimation](#10-storage-estimation)

---

## 1. Data Model Overview

### 1.1 Design Philosophy

The UDM stores all subscriber data directly — there is no separate UDR component.
The relational schema is derived from the 3GPP TS 29.505 specification (Subscription
Data API), which defines 80+ data schemas and 180+ REST endpoints organized
hierarchically under the SUPI (Subscription Permanent Identifier).

The mapping from 3GPP data structures to relational tables follows these principles:

| Principle | Rationale |
|-----------|-----------|
| **SUPI as root key** | Every subscriber record is anchored by SUPI, enabling hash-based sharding |
| **One table per 3GPP data set** | `AM`, `SM`, `SMS`, `EE`, `PP`, `UECM` each map to dedicated tables |
| **JSONB for nested structures** | 3GPP schemas contain deeply nested objects (NSSAI, DNN configs, QoS); JSONB avoids excessive normalization |
| **Flat columns for indexed fields** | Fields used in WHERE clauses or joins are stored as top-level columns |
| **Composite keys for per-slice/DNN data** | Session management data is keyed by `(supi, serving_plmn_id, snssai, dnn)` |
| **Separate tables for context vs. provisioned data** | Registration state (volatile) is separated from subscription profiles (stable) |

### 1.2 3GPP TS 29.505 Data Set Mapping

The TS 29.505 API organizes data under `/subscription-data/{ueId}/` with these
major categories, each mapping to one or more database tables:

| 3GPP Data Set | API Path Prefix | Database Table(s) |
|---------------|-----------------|-------------------|
| Authentication Subscription | `/authentication-data/authentication-subscription` | `authentication_data` |
| Authentication Status | `/authentication-data/authentication-status` | `authentication_status` |
| Access & Mobility Data | `/{servingPlmnId}/provisioned-data/am-data` | `access_mobility_subscription` |
| Session Management Data | `/{servingPlmnId}/provisioned-data/sm-data` | `session_management_subscription` |
| SMF Selection Data | `/{servingPlmnId}/provisioned-data/smf-selection-subscription-data` | `smf_selection_data` |
| SMS Subscription Data | `/{servingPlmnId}/provisioned-data/sms-data` | `sms_subscription_data` |
| SMS Management Data | `/{servingPlmnId}/provisioned-data/sms-mng-data` | `sms_management_data` |
| AMF 3GPP Registration | `/context-data/amf-3gpp-access` | `amf_registrations` |
| AMF Non-3GPP Registration | `/context-data/amf-non-3gpp-access` | `amf_registrations` |
| SMF Registrations | `/context-data/smf-registrations` | `smf_registrations` |
| SMSF Registration | `/context-data/smsf-3gpp-access` | `smsf_registrations` |
| EE Subscriptions | `/context-data/ee-subscriptions` | `ee_subscriptions` |
| SDM Subscriptions | `/context-data/sdm-subscriptions` | `sdm_subscriptions` |
| PP Data | `/pp-data` | `pp_data` |
| PP Profile Data | `/pp-profile-data` | `pp_profile_data` |
| Operator Specific Data | `/operator-specific-data` | `operator_specific_data` |
| Identity Data | `/identity-data` | `subscribers` |
| Shared Data | `/shared-data` | `shared_data` |
| Trace Data | `/{servingPlmnId}/provisioned-data/trace-data` | `trace_data` |
| UE Update Confirmation | `/ue-update-confirmation-data` | `ue_update_confirmation` |
| Network Slice Data | (embedded in AM data / NSSAI) | `network_slice_data` |

### 1.3 Data Lifecycle

```
Provisioning ──► Subscriber Created (auth_data, am_data, sm_data)
                       │
UE Attach ────► Authentication (read auth_data, write auth_status)
                       │
Registration ──► AMF/SMF/SMSF context written (amf/smf/smsf_registrations)
                       │
Session ──────► PDU session data updated (smf_registrations)
                       │
Deregistration ► Context data removed; subscription data persists
                       │
Deletion ─────► All subscriber data purged (cascading delete by SUPI)
```

---

## 2. Entity-Relationship Model

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         UDM Entity-Relationship Diagram                         │
└─────────────────────────────────────────────────────────────────────────────────┘

                          ┌──────────────────────┐
                          │     subscribers      │
                          │──────────────────────│
                          │ PK supi              │
                          │    gpsi              │
                          │    supi_type         │
                          │    identity_data     │
                          └──────────┬───────────┘
                                     │
            ┌────────────────────────┼────────────────────────────┐
            │                        │                            │
            │ 1:1                    │ 1:1                        │ 1:N
            ▼                        ▼                            ▼
┌───────────────────────┐ ┌────────────────────────┐  ┌─────────────────────────┐
│  authentication_data  │ │  network_slice_data    │  │  operator_specific_data │
│───────────────────────│ │────────────────────────│  │─────────────────────────│
│ FK supi               │ │ FK supi                │  │ PK id                   │
│    auth_method        │ │    nssai (JSONB)       │  │ FK supi                 │
│    k_key              │ │    default_nssais      │  │    data_type            │
│    opc_key            │ │    single_nssais       │  │    data_value (JSONB)   │
│    sqn                │ └────────────────────────┘  └─────────────────────────┘
│    amf_value          │
└───────────────────────┘
            │
            │ 1:N
            ▼
┌───────────────────────┐
│ authentication_status │
│───────────────────────│
│ FK supi               │
│    serving_network    │
│    auth_type          │
│    success            │
│    time_stamp         │
└───────────────────────┘

       ┌──────────────────────────┬───────────────────────────────┐
       │                          │                               │
       │ 1:N (per PLMN)          │ 1:N (per PLMN/SNSSAI/DNN)    │ 1:N (per PLMN)
       ▼                          ▼                               ▼
┌──────────────────────┐ ┌───────────────────────────┐ ┌────────────────────────┐
│ access_mobility_     │ │ session_management_       │ │ sms_subscription_data  │
│ subscription         │ │ subscription              │ │────────────────────────│
│──────────────────────│ │───────────────────────────│ │ FK supi                │
│ FK supi              │ │ FK supi                   │ │    serving_plmn_id     │
│    serving_plmn_id   │ │    serving_plmn_id        │ │    sms_subscribed      │
│    subscribed_ue_ambr│ │    single_nssai (JSONB)   │ │    sms_data (JSONB)    │
│    nssai (JSONB)     │ │    dnn_configurations     │ └────────────────────────┘
│    rat_restrictions   │ │    (JSONB)               │
│    rfsp_index        │ │    internal_group_ids     │
└──────────────────────┘ └───────────────────────────┘

       ┌──────────────────────────┬──────────────────────────────┐
       │                          │                              │
       │ 1:N (per access_type)   │ 1:N (per pdu_session_id)    │ 1:N (per access_type)
       ▼                          ▼                              ▼
┌──────────────────────┐ ┌──────────────────────┐  ┌──────────────────────────┐
│  amf_registrations   │ │  smf_registrations   │  │   smsf_registrations     │
│──────────────────────│ │──────────────────────│  │──────────────────────────│
│ FK supi              │ │ FK supi              │  │ FK supi                  │
│    amf_instance_id   │ │    pdu_session_id    │  │    smsf_instance_id      │
│    dereg_callback_uri│ │    smf_instance_id   │  │    access_type           │
│    guami (JSONB)     │ │    dnn               │  │    registration_time     │
│    rat_type          │ │    single_nssai      │  │    plmn_id               │
│    access_type       │ │    plmn_id           │  └──────────────────────────┘
└──────────────────────┘ └──────────────────────┘

       ┌──────────────────────────┬──────────────────────────────┐
       │                          │                              │
       │ 1:N                     │ 1:N                          │ 1:1
       ▼                          ▼                              ▼
┌──────────────────────┐ ┌──────────────────────┐  ┌──────────────────────────┐
│  ee_subscriptions    │ │  sdm_subscriptions   │  │   pp_data                │
│──────────────────────│ │──────────────────────│  │──────────────────────────│
│ PK subscription_id   │ │ PK subscription_id   │  │ FK supi                  │
│    supi / gpsi       │ │ FK supi              │  │    comm_characteristics  │
│    callback_reference│ │    callback_reference│  │    (JSONB)               │
│    monitoring_configs│ │    monitored_resource│  │    expected_ue_behaviour │
│    (JSONB)           │ │    _uris (JSONB)     │  │    (JSONB)               │
└──────────────────────┘ └──────────────────────┘  └──────────────────────────┘

       ┌──────────────────────────┐
       │                          │
       │ Standalone (shared_data_id PK)
       ▼
┌──────────────────────┐
│    shared_data       │
│──────────────────────│
│ PK shared_data_id    │
│    shared_data_type  │
│    data (JSONB)      │
└──────────────────────┘
```

### 2.1 Relationship Summary

| Parent Entity | Child Entity | Cardinality | Join Key |
|---------------|-------------|-------------|----------|
| `subscribers` | `authentication_data` | 1:1 | `supi` |
| `subscribers` | `access_mobility_subscription` | 1:N | `supi` (per PLMN) |
| `subscribers` | `session_management_subscription` | 1:N | `supi` (per PLMN/SNSSAI/DNN) |
| `subscribers` | `sms_subscription_data` | 1:N | `supi` (per PLMN) |
| `subscribers` | `amf_registrations` | 1:N | `supi` (per access type) |
| `subscribers` | `smf_registrations` | 1:N | `supi` (per PDU session) |
| `subscribers` | `smsf_registrations` | 1:N | `supi` (per access type) |
| `subscribers` | `ee_subscriptions` | 1:N | `supi` or `gpsi` |
| `subscribers` | `sdm_subscriptions` | 1:N | `supi` |
| `subscribers` | `pp_data` | 1:1 | `supi` |
| `subscribers` | `network_slice_data` | 1:1 | `supi` |
| `subscribers` | `operator_specific_data` | 1:N | `supi` |
| (standalone) | `shared_data` | — | `shared_data_id` |

---

## 3. YugabyteDB Schema Design

All tables use the YSQL (PostgreSQL-compatible) API. Tables are created in the
`udm` schema within a dedicated `udm_db` database.

### 3.0 Database and Schema Setup

```sql
-- Database creation (run as superuser)
CREATE DATABASE udm_db;

-- Connect to udm_db, then:
CREATE SCHEMA IF NOT EXISTS udm;
SET search_path TO udm, public;

-- Enable required extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
```

### 3.1 subscribers — Core Subscriber Table

```sql
CREATE TABLE udm.subscribers (
    supi                TEXT        NOT NULL,
    gpsi                TEXT,
    supi_type           TEXT        NOT NULL DEFAULT 'imsi'
                                   CHECK (supi_type IN ('imsi', 'nai', 'gci', 'gli')),
    gpsi_type           TEXT        CHECK (gpsi_type IN ('msisdn', 'external_id')),
    group_ids           TEXT[],
    identity_data       JSONB,
    odb_data            JSONB,
    roaming_allowed     BOOLEAN     NOT NULL DEFAULT TRUE,
    provisioning_status TEXT        NOT NULL DEFAULT 'active'
                                   CHECK (provisioning_status IN ('active', 'suspended', 'pending_deletion')),
    shared_data_ids     TEXT[],
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi)
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.subscribers IS
    'Core subscriber table. One row per SUPI. Root entity for all subscription data.';
COMMENT ON COLUMN udm.subscribers.supi IS
    'Subscription Permanent Identifier (e.g., imsi-310260000000001)';
COMMENT ON COLUMN udm.subscribers.gpsi IS
    'Generic Public Subscription Identifier (e.g., msisdn-14155551234)';
```

### 3.2 authentication_data — Auth Credentials

```sql
CREATE TABLE udm.authentication_data (
    supi                        TEXT        NOT NULL,
    auth_method                 TEXT        NOT NULL
                                            CHECK (auth_method IN (
                                                '5G_AKA', 'EAP_AKA_PRIME', 'EAP_TLS',
                                                'EAP_TTLS', 'NONE'
                                            )),
    k_key                       BYTEA,
    opc_key                     BYTEA,
    topc_key                    BYTEA,
    sqn                         TEXT        DEFAULT '000000000000',
    sqn_scheme                  TEXT        DEFAULT 'NON_TIME_BASED'
                                            CHECK (sqn_scheme IN (
                                                'GENERAL', 'NON_TIME_BASED', 'TIME_BASED'
                                            )),
    sqn_last_indexes            JSONB       DEFAULT '{}'::JSONB,
    sqn_ind_length              INTEGER     DEFAULT 5,
    amf_value                   TEXT        DEFAULT '8000',
    algorithm_id                TEXT,
    protection_parameter_id     TEXT,
    vector_generation_in_hss    BOOLEAN     NOT NULL DEFAULT FALSE,
    hss_group_id                TEXT,
    n5gc_auth_method            TEXT,
    rg_authentication_ind       BOOLEAN     NOT NULL DEFAULT FALSE,
    akma_allowed                BOOLEAN     NOT NULL DEFAULT FALSE,
    routing_id                  TEXT,
    nswo_allowed                BOOLEAN     NOT NULL DEFAULT FALSE,
    version                     BIGINT      NOT NULL DEFAULT 1,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_auth_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.authentication_data IS
    'Authentication subscription data per TS 29.505 AuthenticationSubscription schema. '
    'Stores K/OPc keys and SQN state for 5G-AKA and EAP-AKA'' procedures.';
```

### 3.3 authentication_status — Auth Event Log

```sql
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
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.authentication_status IS
    'Stores the result of the most recent authentication attempt per serving network.';
```

### 3.4 access_mobility_subscription — AM Subscription Data

```sql
CREATE TABLE udm.access_mobility_subscription (
    supi                    TEXT        NOT NULL,
    serving_plmn_id         TEXT        NOT NULL DEFAULT '00000',
    gpsis                   TEXT[],
    internal_group_ids      TEXT[],
    shared_data_ids         TEXT[],
    subscribed_ue_ambr      JSONB,
    nssai                   JSONB,
    rat_restrictions        JSONB,
    forbidden_areas         JSONB,
    service_area_restriction JSONB,
    core_network_type_restrictions JSONB,
    rfsp_index              INTEGER,
    subs_reg_timer          INTEGER,
    ue_usage_type           INTEGER,
    mps_priority            BOOLEAN     NOT NULL DEFAULT FALSE,
    mcs_priority            BOOLEAN     NOT NULL DEFAULT FALSE,
    active_time             INTEGER,
    sor_info                JSONB,
    sor_info_expect_ind     BOOLEAN     NOT NULL DEFAULT FALSE,
    soraf_retrieval         BOOLEAN     NOT NULL DEFAULT FALSE,
    sor_update_indicator_list JSONB,
    upu_info                JSONB,
    routing_indicator       TEXT,
    mico_allowed            BOOLEAN     NOT NULL DEFAULT FALSE,
    shared_am_data_ids      TEXT[],
    odb_packet_services     TEXT,
    subscribed_dnn_list     TEXT[],
    service_gap_time        INTEGER,
    trace_data              JSONB,
    cag_data                JSONB,
    wireline_area           JSONB,
    wireline_forbidden_areas JSONB,
    ec_restriction_data     JSONB,
    adjacent_plmn_restrictions JSONB,
    remote_prov_ind         BOOLEAN     NOT NULL DEFAULT FALSE,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_am_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.access_mobility_subscription IS
    'Access and Mobility subscription data per TS 29.505 AccessAndMobilitySubscriptionData. '
    'Scoped per serving PLMN for roaming scenarios.';
```

### 3.5 session_management_subscription — Per-DNN/S-NSSAI SM Data

```sql
CREATE TABLE udm.session_management_subscription (
    supi                    TEXT        NOT NULL,
    serving_plmn_id         TEXT        NOT NULL DEFAULT '00000',
    single_nssai            JSONB       NOT NULL,
    dnn_configurations      JSONB       NOT NULL DEFAULT '{}'::JSONB,
    internal_group_ids      TEXT[],
    shared_data_ids         TEXT[],
    shared_sm_subs_data_ids TEXT[],
    odb_packet_services     TEXT,
    trace_data              JSONB,
    shared_trace_data_id    TEXT,
    expected_ue_behaviours  JSONB,
    suggested_packet_num_dl JSONB,
    three_gpp_charging_characteristics TEXT,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id, single_nssai),
    CONSTRAINT fk_sm_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.session_management_subscription IS
    'Session management subscription data per slice (S-NSSAI). Contains DNN configurations '
    'with QoS profiles, PDU session types, and SSC modes as JSONB.';
COMMENT ON COLUMN udm.session_management_subscription.dnn_configurations IS
    'Map of DNN -> DnnConfiguration objects. Each entry defines PDU session type, '
    'SSC mode, 5G QoS profile, session AMBR, and static IP config.';
```

### 3.6 smf_selection_data — SMF Selection Subscription Data

```sql
CREATE TABLE udm.smf_selection_data (
    supi                    TEXT        NOT NULL,
    serving_plmn_id         TEXT        NOT NULL DEFAULT '00000',
    subscribed_snssai_infos JSONB,
    shared_data_ids         TEXT[],
    shared_snssai_infos     JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_smf_sel_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

COMMENT ON TABLE udm.smf_selection_data IS
    'SMF selection subscription data mapping subscribed S-NSSAI to allowed DNNs.';
```

### 3.7 sms_subscription_data — SMS Subscription Data

```sql
CREATE TABLE udm.sms_subscription_data (
    supi                TEXT        NOT NULL,
    serving_plmn_id     TEXT        NOT NULL DEFAULT '00000',
    sms_subscribed      BOOLEAN     NOT NULL DEFAULT TRUE,
    shared_data_ids     TEXT[],
    sms_data            JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_sms_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;
```

### 3.8 sms_management_data — SMS Management Subscription Data

```sql
CREATE TABLE udm.sms_management_data (
    supi                TEXT        NOT NULL,
    serving_plmn_id     TEXT        NOT NULL DEFAULT '00000',
    mt_sms_subscribed   BOOLEAN     NOT NULL DEFAULT TRUE,
    mt_sms_barring_all  BOOLEAN     NOT NULL DEFAULT FALSE,
    mt_sms_barring_roaming BOOLEAN  NOT NULL DEFAULT FALSE,
    mo_sms_subscribed   BOOLEAN     NOT NULL DEFAULT TRUE,
    mo_sms_barring_all  BOOLEAN     NOT NULL DEFAULT FALSE,
    mo_sms_barring_roaming BOOLEAN  NOT NULL DEFAULT FALSE,
    shared_data_ids     TEXT[],
    trace_data          JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, serving_plmn_id),
    CONSTRAINT fk_sms_mng_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;
```

### 3.9 amf_registrations — AMF Context Registrations

```sql
CREATE TABLE udm.amf_registrations (
    supi                    TEXT        NOT NULL,
    access_type             TEXT        NOT NULL
                                       CHECK (access_type IN (
                                           '3GPP_ACCESS', 'NON_3GPP_ACCESS'
                                       )),
    amf_instance_id         UUID        NOT NULL,
    dereg_callback_uri      TEXT        NOT NULL,
    amf_service_name_dereg  TEXT,
    pcscf_restoration_callback_uri TEXT,
    amf_service_name_pcscf_rest TEXT,
    guami                   JSONB       NOT NULL,
    rat_type                TEXT        NOT NULL,
    ue_reachable            BOOLEAN     NOT NULL DEFAULT TRUE,
    initial_registration_ind BOOLEAN    NOT NULL DEFAULT FALSE,
    emergency_registration_ind BOOLEAN  NOT NULL DEFAULT FALSE,
    ims_vo_ps               TEXT        CHECK (ims_vo_ps IN (
                                           'HOMOGENEOUS_SUPPORT',
                                           'HOMOGENEOUS_NON_SUPPORT',
                                           'NON_HOMOGENEOUS_OR_UNKNOWN'
                                       )),
    plmn_id                 JSONB,
    backup_amf_info         JSONB,
    dr_flag                 BOOLEAN     NOT NULL DEFAULT FALSE,
    supi_pei_available      BOOLEAN     NOT NULL DEFAULT FALSE,
    ue_srvcc_capability     BOOLEAN     NOT NULL DEFAULT FALSE,
    registration_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    no_ee_subscription_ind  BOOLEAN     NOT NULL DEFAULT FALSE,
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, access_type),
    CONSTRAINT fk_amf_reg_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.amf_registrations IS
    'AMF registration context per TS 29.503 UECM service. Stores both 3GPP and non-3GPP '
    'access registrations distinguished by access_type. Updated on every AMF registration.';
```

### 3.10 smf_registrations — SMF PDU Session Registrations

```sql
CREATE TABLE udm.smf_registrations (
    supi                    TEXT        NOT NULL,
    pdu_session_id          INTEGER     NOT NULL CHECK (pdu_session_id BETWEEN 1 AND 255),
    smf_instance_id         UUID        NOT NULL,
    smf_set_id              TEXT,
    smf_service_instance_id TEXT,
    dnn                     TEXT        NOT NULL,
    single_nssai            JSONB       NOT NULL,
    plmn_id                 JSONB       NOT NULL,
    emergency_services      BOOLEAN     NOT NULL DEFAULT FALSE,
    pcscf_restoration_callback_uri TEXT,
    pdu_session_type        TEXT        CHECK (pdu_session_type IN (
                                           'IPV4', 'IPV6', 'IPV4V6',
                                           'UNSTRUCTURED', 'ETHERNET'
                                       )),
    pgw_fqdn                TEXT,
    registration_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    registration_reason     TEXT        CHECK (registration_reason IN (
                                           'INITIAL_REGISTRATION',
                                           'CHANGE_REGISTRATION'
                                       )),
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, pdu_session_id),
    CONSTRAINT fk_smf_reg_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 128 TABLETS;

COMMENT ON TABLE udm.smf_registrations IS
    'SMF registration context per PDU session. Multiple active sessions per subscriber '
    'with unique pdu_session_id (1-255).';
```

### 3.11 smsf_registrations — SMSF Registrations

```sql
CREATE TABLE udm.smsf_registrations (
    supi                    TEXT        NOT NULL,
    access_type             TEXT        NOT NULL
                                       CHECK (access_type IN (
                                           '3GPP_ACCESS', 'NON_3GPP_ACCESS'
                                       )),
    smsf_instance_id        UUID        NOT NULL,
    smsf_set_id             TEXT,
    smsf_service_instance_id TEXT,
    plmn_id                 JSONB       NOT NULL,
    smsf_map_address        TEXT,
    ue_reachable            BOOLEAN     NOT NULL DEFAULT TRUE,
    registration_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi, access_type),
    CONSTRAINT fk_smsf_reg_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;
```

### 3.12 ee_subscriptions — Event Exposure Subscriptions

```sql
CREATE TABLE udm.ee_subscriptions (
    subscription_id         TEXT        NOT NULL DEFAULT gen_random_uuid()::TEXT,
    supi                    TEXT,
    gpsi                    TEXT,
    ue_group_id             TEXT,
    callback_reference      TEXT        NOT NULL,
    monitoring_configurations JSONB     NOT NULL DEFAULT '{}'::JSONB,
    reporting_options        JSONB,
    supported_features      TEXT,
    subscription_data_subscriptions JSONB,
    scef_id                 TEXT,
    nf_instance_id          UUID,
    data_restoration_callback_uri TEXT,
    excluded_unsubscribed_ues BOOLEAN   NOT NULL DEFAULT FALSE,
    immediate_report_data   JSONB,
    expiry_time             TIMESTAMPTZ,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (subscription_id),
    CONSTRAINT fk_ee_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE,
    CONSTRAINT chk_ee_identity
        CHECK (supi IS NOT NULL OR gpsi IS NOT NULL OR ue_group_id IS NOT NULL)
) SPLIT INTO 64 TABLETS;

COMMENT ON TABLE udm.ee_subscriptions IS
    'Event Exposure subscriptions per TS 29.503 Nudm_EE. Supports per-UE (SUPI/GPSI) '
    'and per-group subscriptions. monitoring_configurations JSONB contains monitoring '
    'type, event filters, and reporting thresholds.';
COMMENT ON COLUMN udm.ee_subscriptions.monitoring_configurations IS
    'Map of monitoring config ID -> MonitoringConfiguration. Each entry specifies '
    'event type (LOSS_OF_CONNECTIVITY, UE_REACHABILITY, LOCATION_REPORTING, etc.), '
    'maximum number of reports, and monitoring duration.';
```

### 3.13 sdm_subscriptions — SDM Change Notification Subscriptions

```sql
CREATE TABLE udm.sdm_subscriptions (
    subscription_id         TEXT        NOT NULL DEFAULT gen_random_uuid()::TEXT,
    supi                    TEXT        NOT NULL,
    callback_reference      TEXT        NOT NULL,
    monitored_resource_uris TEXT[]      NOT NULL,
    nf_instance_id          UUID        NOT NULL,
    implicit_unsubscribe    BOOLEAN     NOT NULL DEFAULT FALSE,
    supported_features      TEXT,
    expiry_time             TIMESTAMPTZ,
    single_nssai            JSONB,
    dnn                     TEXT,
    plmn_id                 JSONB,
    immediate_report        BOOLEAN     NOT NULL DEFAULT FALSE,
    data_restoration_callback_uri TEXT,
    unique_subscription     BOOLEAN     NOT NULL DEFAULT FALSE,
    report                  JSONB,
    context_info            JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (subscription_id),
    CONSTRAINT fk_sdm_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

COMMENT ON TABLE udm.sdm_subscriptions IS
    'SDM notification subscriptions per TS 29.503 Nudm_SDM Subscribe/Unsubscribe. '
    'NF consumers (AMF, SMF) subscribe to be notified when subscription data changes.';
```

### 3.14 pp_data — Parameter Provisioning Data

```sql
CREATE TABLE udm.pp_data (
    supi                            TEXT        NOT NULL,
    communication_characteristics   JSONB,
    supported_features              TEXT,
    expected_ue_behaviour           JSONB,
    ec_restriction                  JSONB,
    acs_info                        JSONB,
    sor_info                        JSONB,
    five_mbs_authorization_info     JSONB,
    steering_container              JSONB,
    pp_dl_packet_count              INTEGER,
    pp_dl_packet_count_ext          JSONB,
    pp_maximum_response_time        INTEGER,
    pp_maximum_latency              INTEGER,
    version                         BIGINT      NOT NULL DEFAULT 1,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_pp_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

COMMENT ON TABLE udm.pp_data IS
    'Parameter Provisioning data per TS 29.503 Nudm_PP. Stores communication '
    'characteristics and expected UE behaviour provisioned by NEF/AF.';
```

### 3.15 pp_profile_data — PP Profile Data

```sql
CREATE TABLE udm.pp_profile_data (
    supi                            TEXT        NOT NULL,
    allowed_mtc_providers           JSONB,
    supported_features              TEXT,
    version                         BIGINT      NOT NULL DEFAULT 1,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_pp_profile_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;
```

### 3.16 network_slice_data — Slice-Specific Subscription Data

```sql
CREATE TABLE udm.network_slice_data (
    supi                    TEXT        NOT NULL,
    nssai                   JSONB       NOT NULL DEFAULT '[]'::JSONB,
    default_single_nssais   JSONB       NOT NULL DEFAULT '[]'::JSONB,
    single_nssais           JSONB       NOT NULL DEFAULT '[]'::JSONB,
    mapping_of_nssai        JSONB,
    suppressed_nssai        JSONB,
    version                 BIGINT      NOT NULL DEFAULT 1,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_nsd_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 64 TABLETS;

COMMENT ON TABLE udm.network_slice_data IS
    'Network slice subscription data. nssai JSONB stores the full Nssai structure: '
    'array of {sst: int, sd: string} objects representing allowed S-NSSAIs.';
COMMENT ON COLUMN udm.network_slice_data.nssai IS
    'Full NSSAI: [{"sst": 1, "sd": "000001"}, {"sst": 2, "sd": "000002"}]';
COMMENT ON COLUMN udm.network_slice_data.default_single_nssais IS
    'Default S-NSSAIs the UE uses when no specific request is made.';
```

### 3.17 operator_specific_data — Extensible Operator Data

```sql
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
) SPLIT INTO 64 TABLETS;

COMMENT ON TABLE udm.operator_specific_data IS
    'Extensible key-value store for operator-defined data per TS 29.505 '
    'OperatorSpecificDataContainer. Each data_type is a unique operator-defined key.';
```

### 3.18 shared_data — Shared Subscription Profiles

```sql
CREATE TABLE udm.shared_data (
    shared_data_id      TEXT        NOT NULL,
    shared_data_type    TEXT        NOT NULL,
    data                JSONB       NOT NULL,
    description         TEXT,
    supported_features  TEXT,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (shared_data_id)
) SPLIT INTO 16 TABLETS;

COMMENT ON TABLE udm.shared_data IS
    'Shared subscription data profiles referenced by shared_data_ids in per-subscriber '
    'tables. Reduces storage duplication for common subscription configurations.';
```

### 3.19 ue_update_confirmation — UE Update Confirmation Data

```sql
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
```

### 3.20 trace_data — Subscriber Trace Configuration

```sql
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
) SPLIT INTO 16 TABLETS;
```

### 3.21 ip_sm_gw_registrations — IP-SM-GW Context

```sql
CREATE TABLE udm.ip_sm_gw_registrations (
    supi                TEXT        NOT NULL,
    ip_sm_gw_map_address TEXT,
    unri_indicator      BOOLEAN     NOT NULL DEFAULT FALSE,
    reset_ids           TEXT[],
    context_info        JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_ipsmgw_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 16 TABLETS;
```

### 3.22 message_waiting_data — Message Waiting Indication

```sql
CREATE TABLE udm.message_waiting_data (
    supi                TEXT        NOT NULL,
    mwd_list            JSONB       NOT NULL DEFAULT '[]'::JSONB,
    version             BIGINT      NOT NULL DEFAULT 1,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (supi),
    CONSTRAINT fk_mwd_subscriber
        FOREIGN KEY (supi) REFERENCES udm.subscribers(supi) ON DELETE CASCADE
) SPLIT INTO 16 TABLETS;
```

### 3.23 audit_log — Change Audit Trail

```sql
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
) SPLIT INTO 32 TABLETS;

COMMENT ON TABLE udm.audit_log IS
    'Append-only audit trail for all subscription data changes. Uses range sharding '
    'on auto-increment ID for time-ordered queries.';
```

---

### 3.24 suci_profiles — SUCI De-concealment Key Profiles

```sql
-- Phase 4: SUCI de-concealment home network key profiles
-- Based on: docs/security.md §4.3 (Home Network Key Management)
-- 3GPP: TS 33.501 §6.12 — SUCI de-concealment, ECIES Profile A and B

CREATE TABLE udm.suci_profiles (
    hn_key_id       INTEGER         NOT NULL CHECK (hn_key_id >= 0 AND hn_key_id <= 255),
    profile_type    TEXT            NOT NULL CHECK (profile_type IN ('A', 'B')),
    public_key      BYTEA           NOT NULL,
    hsm_key_ref     TEXT            NOT NULL,
    is_active       BOOLEAN         NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    updated_at      TIMESTAMPTZ     NOT NULL DEFAULT NOW(),

    PRIMARY KEY (hn_key_id)
);
```

| Column | Type | Description |
|--------|------|-------------|
| `hn_key_id` | `INTEGER` | Home Network Public Key Identifier (0–255 per TS 23.003) |
| `profile_type` | `TEXT` | ECIES profile: `'A'` (X25519) or `'B'` (secp256r1) |
| `public_key` | `BYTEA` | Raw public key bytes (32 bytes Profile A, 33 bytes compressed Profile B) |
| `hsm_key_ref` | `TEXT` | HSM key reference/handle — private key never leaves HSM boundary (docs/security.md §4.3) |
| `is_active` | `BOOLEAN` | Whether this key is currently active for de-concealment |
| `created_at` | `TIMESTAMPTZ` | Key creation timestamp |
| `expires_at` | `TIMESTAMPTZ` | Optional key expiration timestamp for rotation |
| `updated_at` | `TIMESTAMPTZ` | Last modification timestamp |

**Design notes:**
- **No FK to subscribers** — this is infrastructure configuration, not per-subscriber data
- **No SPLIT INTO** — small table (few rows, not subscriber-partitioned)
- **HSM key reference** — per `docs/security.md` §4.3/§4.4, private key material must not be stored in the database or loaded into application memory. The `hsm_key_ref` column stores an HSM handle/identifier used by the UEID service to delegate ECIES de-concealment to the HSM boundary

---

## 4. Indexing Strategy

### 4.1 Primary Indexes

All primary keys use YugabyteDB hash-based sharding by default, which
distributes data evenly across tablets based on the hash of the SUPI:

```sql
-- Primary keys are implicitly indexed. YugabyteDB hash-shards on the
-- first column of the PK by default, giving O(1) point lookups by SUPI.
-- Composite PKs like (supi, serving_plmn_id) hash on supi and range on
-- the remaining columns within the same tablet.
```

### 4.2 Secondary Indexes for Lookup Patterns

```sql
-- GPSI (MSISDN) lookups: resolve MSISDN -> SUPI
CREATE INDEX idx_subscribers_gpsi
    ON udm.subscribers (gpsi)
    WHERE gpsi IS NOT NULL
    SPLIT INTO 64 TABLETS;

-- Group-based queries for EE subscriptions
CREATE INDEX idx_ee_subs_group
    ON udm.ee_subscriptions (ue_group_id)
    WHERE ue_group_id IS NOT NULL
    SPLIT INTO 16 TABLETS;

-- GPSI-based EE subscription lookups
CREATE INDEX idx_ee_subs_gpsi
    ON udm.ee_subscriptions (gpsi)
    WHERE gpsi IS NOT NULL
    SPLIT INTO 16 TABLETS;

-- EE subscriptions by SUPI
CREATE INDEX idx_ee_subs_supi
    ON udm.ee_subscriptions (supi)
    WHERE supi IS NOT NULL
    SPLIT INTO 32 TABLETS;

-- SDM subscriptions by SUPI
CREATE INDEX idx_sdm_subs_supi
    ON udm.sdm_subscriptions (supi)
    SPLIT INTO 32 TABLETS;

-- AMF instance lookups (find all subscribers registered to a specific AMF)
CREATE INDEX idx_amf_reg_instance
    ON udm.amf_registrations (amf_instance_id)
    SPLIT INTO 32 TABLETS;

-- SMF instance lookups
CREATE INDEX idx_smf_reg_instance
    ON udm.smf_registrations (smf_instance_id)
    SPLIT INTO 32 TABLETS;

-- SMF registration by DNN and slice (slice-aware queries)
CREATE INDEX idx_smf_reg_dnn_nssai
    ON udm.smf_registrations (dnn, single_nssai)
    SPLIT INTO 32 TABLETS;

-- Operator-specific data by type
CREATE INDEX idx_opdata_supi
    ON udm.operator_specific_data (supi)
    SPLIT INTO 32 TABLETS;

-- Audit log: time-based queries and per-subscriber audit
CREATE INDEX idx_audit_supi_time
    ON udm.audit_log (supi, created_at DESC)
    SPLIT INTO 32 TABLETS;

CREATE INDEX idx_audit_time
    ON udm.audit_log (created_at DESC)
    SPLIT INTO 32 TABLETS;
```

### 4.3 Covering Indexes for Hot Paths

```sql
-- SDM hot path: AMF retrieves AM data for a subscriber.
-- Covering index avoids table lookup for the most common fields.
CREATE INDEX idx_am_data_covering
    ON udm.access_mobility_subscription (supi, serving_plmn_id)
    INCLUDE (nssai, gpsis, subscribed_ue_ambr, rat_restrictions, rfsp_index)
    SPLIT INTO 64 TABLETS;

-- Authentication hot path: UEAU reads auth method and key references.
CREATE INDEX idx_auth_covering
    ON udm.authentication_data (supi)
    INCLUDE (auth_method, algorithm_id, sqn, amf_value)
    SPLIT INTO 64 TABLETS;

-- AMF registration hot path: SDM checks if subscriber is registered.
CREATE INDEX idx_amf_reg_covering
    ON udm.amf_registrations (supi)
    INCLUDE (amf_instance_id, rat_type, access_type, dereg_callback_uri)
    SPLIT INTO 64 TABLETS;

-- SM data hot path: SMF retrieves DNN configs for a subscriber/PLMN/slice.
CREATE INDEX idx_sm_data_covering
    ON udm.session_management_subscription (supi, serving_plmn_id)
    INCLUDE (single_nssai, dnn_configurations)
    SPLIT INTO 64 TABLETS;
```

### 4.4 JSONB Indexes for Nested Queries

```sql
-- GIN index on NSSAI for slice-based queries
CREATE INDEX idx_am_nssai_gin
    ON udm.access_mobility_subscription USING GIN (nssai);

-- GIN index on monitoring configurations for event type filtering
CREATE INDEX idx_ee_monitoring_gin
    ON udm.ee_subscriptions USING GIN (monitoring_configurations);

-- GIN index on DNN configurations
CREATE INDEX idx_sm_dnn_configs_gin
    ON udm.session_management_subscription USING GIN (dnn_configurations);
```

---

## 5. Data Partitioning and Sharding Strategy

### 5.1 Hash Sharding by SUPI

All subscriber-anchored tables use SUPI as the hash partition key. YugabyteDB
automatically distributes data across tablets based on the hash of the SUPI value:

```
SUPI: imsi-310260000000001
  │
  ├── hash(supi) ──► tablet 47 (of 128)
  │
  └── All related rows (auth, AM, SM, registrations) for this SUPI
      land in the SAME tablet, enabling single-tablet transactions.
```

**Configuration per table**:

| Table | Initial Tablets | Auto-Split Threshold | Rationale |
|-------|----------------|---------------------|-----------|
| `subscribers` | 128 | 4 GB | Core table, highest row count |
| `authentication_data` | 128 | 4 GB | 1:1 with subscribers, same access pattern |
| `access_mobility_subscription` | 128 | 4 GB | High read frequency from AMF |
| `session_management_subscription` | 128 | 4 GB | Multiple rows per subscriber (per slice) |
| `amf_registrations` | 128 | 4 GB | High write frequency during attach/handover |
| `smf_registrations` | 128 | 4 GB | Multiple PDU sessions per subscriber |
| `smsf_registrations` | 64 | 4 GB | Lower cardinality (max 2 per subscriber) |
| `ee_subscriptions` | 64 | 4 GB | Fewer subscribers have EE subs |
| `sdm_subscriptions` | 64 | 4 GB | One per NF consumer per subscriber |
| `pp_data` | 64 | 4 GB | 1:1, not all subscribers have PP data |
| `shared_data` | 16 | 2 GB | Small table, few thousand rows |
| `audit_log` | 32 | 4 GB | Range-sharded on ID, not hash |

### 5.2 Range Sharding for Time-Series Data

The `audit_log` table uses range sharding on the auto-increment `id` column
(which correlates with insertion time). This keeps recent entries co-located for
efficient time-range queries:

```sql
-- audit_log PK is (id ASC), which YugabyteDB range-shards by default
-- for ascending integer keys. Old data can be archived by deleting
-- ranges: DELETE FROM audit_log WHERE created_at < NOW() - INTERVAL '90 days';
```

### 5.3 Tablet Splitting Configuration

```sql
-- YugabyteDB cluster-level configuration (set via yb-tserver flags):
--   --tablet_split_low_phase_shard_count_per_node=8
--   --tablet_split_high_phase_shard_count_per_node=24
--   --tablet_split_low_phase_size_threshold_bytes=536870912   (512 MB)
--   --tablet_split_high_phase_size_threshold_bytes=4294967296  (4 GB)
--   --tablet_force_split_threshold_bytes=10737418240           (10 GB)
--   --enable_automatic_tablet_splitting=true
```

### 5.4 Cross-Region Data Placement Policies

For multi-region deployments, YugabyteDB tablespaces control data placement:

```sql
-- Define tablespaces for geo-aware data placement
CREATE TABLESPACE ts_us WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-west-2", "zone": "us-west-2a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "eu-west-1", "zone": "eu-west-1a", "min_num_replicas": 1}
    ]}'
);

-- Optional: region-local tablespace for latency-sensitive data
CREATE TABLESPACE ts_us_local WITH (
    replica_placement = '{"num_replicas": 3, "placement_blocks": [
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1a", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1b", "min_num_replicas": 1},
        {"cloud": "aws", "region": "us-east-1", "zone": "us-east-1c", "min_num_replicas": 1}
    ]}'
);

-- Geo-partitioned table example for region-local auth data
-- (advanced: partition subscribers by PLMN prefix for data sovereignty)
CREATE TABLE udm.auth_data_geo (
    supi TEXT NOT NULL,
    region TEXT NOT NULL,
    auth_method TEXT NOT NULL,
    k_key BYTEA,
    PRIMARY KEY (supi)
) PARTITION BY LIST (region);

CREATE TABLE udm.auth_data_us PARTITION OF udm.auth_data_geo
    FOR VALUES IN ('us') TABLESPACE ts_us_local;
```

### 5.5 Co-Location Strategy

Small, infrequently accessed tables are colocated into a single tablet group
to reduce per-tablet overhead:

```sql
-- Create a colocated database for small reference tables
-- (alternative: use colocated tables within the main database)
CREATE DATABASE udm_ref WITH COLOCATION = true;

-- Within udm_ref, small tables share tablets:
-- shared_data, trace_data, ip_sm_gw_registrations, message_waiting_data
-- These tables typically have <100K rows and benefit from colocation.
```

---

## 6. YugabyteDB-Specific Optimizations

### 6.1 YSQL API Usage

The UDM uses YugabyteDB's YSQL API (PostgreSQL-compatible wire protocol),
which provides full SQL semantics including:

- **ACID transactions** with distributed commit
- **JSONB** data type with GIN indexing
- **CTEs, window functions, and subqueries** for complex queries
- **LISTEN/NOTIFY** for cache invalidation (supplemented by Redis pub/sub)
- **Prepared statements** for reduced parse overhead on hot paths

### 6.2 Tablet Configuration for Telecom Workloads

```yaml
# yb-tserver flags optimized for UDM workload
tserver_flags:
  # Memory: allocate 70% to data block cache for read-heavy workload
  db_block_cache_size_percentage: 70

  # Compaction: level-based for stable write amplification
  tablet_server_svc_queue_length: 500

  # RPC: tune for high-concurrency UDM traffic
  rpc_workers_limit: 256
  svc_queue_length_default: 1000

  # Writes: batch for throughput on registration storms
  ysql_max_write_restart_attempts: 20

  # Reads: optimize for point lookups (dominant UDM pattern)
  ysql_enable_packed_row: true
  ysql_enable_packed_row_for_colocated_table: true

  # Clock: tighten clock skew for distributed transactions
  max_clock_skew_usec: 250000  # 250ms (default: 500ms)
```

### 6.3 JSONB for Flexible/Nested 3GPP Structures

3GPP data models contain deeply nested structures that would require excessive
normalization in pure relational form. JSONB is used for:

| Column | Table | Example JSONB Structure |
|--------|-------|------------------------|
| `nssai` | `access_mobility_subscription` | `[{"sst": 1, "sd": "000001"}, {"sst": 2}]` |
| `dnn_configurations` | `session_management_subscription` | `{"internet": {"pduSessionTypes": {"defaultSessionType": "IPV4"}, "sscModes": {"defaultSscMode": "SSC_MODE_1"}, "5gQosProfile": {"5qi": 9}, "sessionAmbr": {"uplink": "1 Gbps", "downlink": "2 Gbps"}}}` |
| `guami` | `amf_registrations` | `{"plmnId": {"mcc": "310", "mnc": "260"}, "amfId": "020040"}` |
| `single_nssai` | `smf_registrations` | `{"sst": 1, "sd": "000001"}` |
| `monitoring_configurations` | `ee_subscriptions` | `{"1": {"eventType": "LOSS_OF_CONNECTIVITY", "maxReports": 1}, "2": {"eventType": "UE_REACHABILITY"}}` |
| `subscribed_ue_ambr` | `access_mobility_subscription` | `{"uplink": "500 Mbps", "downlink": "1 Gbps"}` |
| `sqn_last_indexes` | `authentication_data` | `{"ausf": 42, "seaf": 41}` |

**JSONB query examples**:

```sql
-- Find subscribers with a specific S-NSSAI
SELECT supi FROM udm.access_mobility_subscription
WHERE nssai @> '[{"sst": 1, "sd": "000001"}]';

-- Get a specific DNN configuration
SELECT dnn_configurations->'internet'->>'pduSessionTypes'
FROM udm.session_management_subscription
WHERE supi = 'imsi-310260000000001'
  AND serving_plmn_id = '31026';

-- Find EE subscriptions monitoring location
SELECT subscription_id FROM udm.ee_subscriptions
WHERE monitoring_configurations @> '{"1": {"eventType": "LOCATION_REPORTING"}}';
```

### 6.4 Connection Pooling

Each UDM microservice pod maintains a `pgxpool` connection pool:

```go
// Connection pool configuration (per service pod)
// pgxpool.Config:
//   MaxConns:          50    // Max open connections per pod
//   MinConns:          25    // Warm idle connections
//   MaxConnLifetime:   30m   // Prevent stale connections
//   MaxConnIdleTime:   5m    // Release unused connections
//   HealthCheckPeriod: 30s   // Proactive broken-connection detection
```

For deployments exceeding 10,000 total connections per region, a PgBouncer or
Odyssey connection pooler is deployed between service pods and YugabyteDB:

```
Service Pods (50 conn each × 300 pods = 15,000 logical connections)
        │
        ▼
   PgBouncer (transaction-mode pooling)
        │ (multiplexed to 3,000 physical connections)
        ▼
   YugabyteDB TServer (per-node connection limit: 300)
```

---

## 7. Transaction Strategy

### 7.1 Single-Row Transactions (Dominant Pattern)

Most UDM operations are single-row reads or writes, which YugabyteDB executes
as fast-path single-tablet transactions without distributed coordination:

| Operation | Transaction Type | Latency Target |
|-----------|-----------------|----------------|
| GET auth subscription by SUPI | Single-row read | < 2 ms |
| GET AM data by SUPI + PLMN | Single-row read | < 2 ms |
| PUT AMF registration | Single-row upsert | < 5 ms |
| PUT SMF registration | Single-row upsert | < 5 ms |
| PATCH auth subscription (SQN update) | Single-row update | < 5 ms |
| DELETE SMF registration | Single-row delete | < 3 ms |

```sql
-- Example: single-row read (no explicit transaction needed)
SELECT auth_method, k_key, opc_key, sqn, amf_value
FROM udm.authentication_data
WHERE supi = $1;

-- Example: single-row upsert for AMF registration
INSERT INTO udm.amf_registrations (supi, access_type, amf_instance_id,
    dereg_callback_uri, guami, rat_type)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (supi, access_type) DO UPDATE SET
    amf_instance_id = EXCLUDED.amf_instance_id,
    dereg_callback_uri = EXCLUDED.dereg_callback_uri,
    guami = EXCLUDED.guami,
    rat_type = EXCLUDED.rat_type,
    updated_at = NOW(),
    version = udm.amf_registrations.version + 1;
```

### 7.2 Multi-Row Transactions (Registration Flows)

Registration and provisioning flows require atomicity across multiple tables:

```sql
-- Initial subscriber provisioning: atomic insert across tables
BEGIN;
    INSERT INTO udm.subscribers (supi, gpsi, supi_type)
    VALUES ('imsi-310260000000001', 'msisdn-14155551234', 'imsi');

    INSERT INTO udm.authentication_data (supi, auth_method, k_key, opc_key, sqn)
    VALUES ('imsi-310260000000001', '5G_AKA', $k, $opc, '000000000000');

    INSERT INTO udm.access_mobility_subscription (supi, serving_plmn_id, nssai,
        subscribed_ue_ambr)
    VALUES ('imsi-310260000000001', '31026', $nssai, $ambr);

    INSERT INTO udm.session_management_subscription (supi, serving_plmn_id,
        single_nssai, dnn_configurations)
    VALUES ('imsi-310260000000001', '31026', $snssai, $dnn_configs);

    INSERT INTO udm.network_slice_data (supi, nssai, default_single_nssais)
    VALUES ('imsi-310260000000001', $nssai, $default_nssais);
COMMIT;
```

```sql
-- Deregistration: atomic cleanup across registration tables
BEGIN;
    DELETE FROM udm.amf_registrations
    WHERE supi = $1 AND access_type = $2;

    DELETE FROM udm.smf_registrations
    WHERE supi = $1;

    DELETE FROM udm.smsf_registrations
    WHERE supi = $1 AND access_type = $2;
COMMIT;
```

### 7.3 Distributed Transaction Handling

For multi-row transactions spanning multiple tablets, YugabyteDB uses a
distributed two-phase commit protocol internally. The UDM application layer
handles this transparently — no application-level 2PC is needed.

**Performance impact**: Multi-row distributed transactions add ~2-5 ms latency
compared to single-row operations. This is acceptable for provisioning flows
(which are infrequent) but registration updates are designed as single-row
upserts to avoid this overhead.

### 7.4 Optimistic vs. Pessimistic Locking

| Scenario | Locking Strategy | Implementation |
|----------|-----------------|----------------|
| **Subscription data reads** | No locking | Plain SELECT (snapshot isolation) |
| **Registration upserts** | Optimistic (version check) | `UPDATE ... WHERE version = $expected_version` |
| **SQN updates** | Optimistic (CAS) | `UPDATE auth SET sqn = $new WHERE sqn = $old` |
| **Provisioning writes** | Pessimistic (SELECT FOR UPDATE) | Rare bulk updates to subscriber profiles |
| **EE subscription create** | Optimistic | `INSERT ... ON CONFLICT DO NOTHING` + retry |

```sql
-- Optimistic locking example: version-based update
UPDATE udm.access_mobility_subscription
SET nssai = $new_nssai,
    version = version + 1,
    updated_at = NOW()
WHERE supi = $1
  AND serving_plmn_id = $2
  AND version = $expected_version;
-- If rows_affected = 0, another writer won — retry with fresh read.
```

### 7.5 Isolation Levels

| Use Case | Isolation Level | Rationale |
|----------|----------------|-----------|
| **SDM reads** | Read Committed (default) | Sufficient for subscription data reads; lower overhead |
| **Auth SQN updates** | Serializable | Prevent SQN reuse across concurrent auth attempts |
| **Registration upserts** | Read Committed | Single-row upserts are inherently serialized |
| **Provisioning transactions** | Serializable | Multi-table consistency during subscriber creation |
| **Audit log inserts** | Read Committed | Append-only; no conflict possible |

```sql
-- Serializable transaction for SQN update during authentication
BEGIN ISOLATION LEVEL SERIALIZABLE;
    SELECT sqn, sqn_last_indexes FROM udm.authentication_data
    WHERE supi = $1 FOR UPDATE;

    -- Application computes new SQN

    UPDATE udm.authentication_data
    SET sqn = $new_sqn, sqn_last_indexes = $new_indexes, updated_at = NOW()
    WHERE supi = $1;
COMMIT;
```

---

## 8. Consistency Model

### 8.1 Strong Consistency for Authentication Data

Authentication data requires strict consistency — a stale SQN can cause
authentication failures or replay attacks:

- **Write path**: All writes go through the Raft leader. Raft replicates to
  a quorum (2 of 3 replicas) before acknowledging the write.
- **Read path**: Reads are served from the Raft leader by default, guaranteeing
  the most recent committed value.
- **Cross-region**: A write in Region A is visible in Region B after Raft
  replication completes (~10-50 ms cross-region). Authentication is always
  performed in the region that holds the tablet leader.

### 8.2 Tunable Consistency for Subscription Data Reads

For read-heavy subscription data (AM, SM, SMS), the UDM can trade freshness
for latency using YugabyteDB's consistency tuning:

```sql
-- Strong read (default): reads from Raft leader
SELECT * FROM udm.access_mobility_subscription WHERE supi = $1;

-- Follower read: reads from nearest replica, may be slightly stale
-- Set at session level for SDM read-heavy operations
SET yb_read_from_followers = true;
SET yb_follower_read_staleness_ms = 5000;  -- allow up to 5 seconds staleness

SELECT * FROM udm.access_mobility_subscription WHERE supi = $1;

-- Reset for operations requiring strong consistency
SET yb_read_from_followers = false;
```

### 8.3 Follower Reads for Read-Heavy Workloads

The SDM service processes 50K-200K reads per second. Follower reads offload
the Raft leader and reduce cross-region latency:

```
Region US-East (Raft Leader)     Region US-West (Follower)     Region EU (Follower)
┌────────────────────┐           ┌────────────────────┐        ┌────────────────────┐
│  Tablet Leader     │──Raft──►  │  Tablet Follower   │  ◄──── │  Tablet Follower   │
│  Serves writes     │  repl     │  Serves local reads│  Raft  │  Serves local reads│
│  + strong reads    │           │  (5s staleness OK) │  repl  │  (5s staleness OK) │
└────────────────────┘           └────────────────────┘        └────────────────────┘
```

**When follower reads are safe**:

| Data Type | Follower Read Safe? | Rationale |
|-----------|-------------------|-----------|
| AM subscription data | ✅ Yes (5s staleness) | Profile changes are rare; NFs cache anyway |
| SM subscription data | ✅ Yes (5s staleness) | DNN configs change infrequently |
| Authentication data | ❌ No — always strong | SQN must be current to prevent replay |
| Registration context | ❌ No — always strong | Stale context = misrouted traffic |
| EE subscriptions | ✅ Yes (5s staleness) | Eventual delivery acceptable |
| Shared data | ✅ Yes (30s staleness) | Reference data, rarely changes |

### 8.4 Cross-Region Consistency Guarantees

YugabyteDB provides the following guarantees across regions:

1. **Linearizability**: All writes are linearizable (appear in a single global
   order consistent with real-time). This is guaranteed by Raft consensus.

2. **Read-your-writes**: Within a single connection/session, a read after a
   write always sees the written value (even across regions, when reading
   from the leader).

3. **Bounded staleness**: Follower reads have configurable staleness bounds.
   The UDM sets this to 5 seconds for subscription data and 0 (disabled)
   for auth and registration data.

4. **Causal consistency**: Achieved within a single session. Cross-session
   causal consistency requires application-level sequencing (e.g., version
   vectors on SDM notification callbacks).

---

## 9. Data Migration and Versioning

### 9.1 Schema Migration Strategy

Schema migrations are managed using [golang-migrate](https://github.com/golang-migrate/migrate)
with the PostgreSQL driver (compatible with YSQL):

```
migrations/
├── 000001_create_subscribers.up.sql
├── 000001_create_subscribers.down.sql
├── 000002_create_authentication_data.up.sql
├── 000002_create_authentication_data.down.sql
├── 000003_create_access_mobility.up.sql
├── 000003_create_access_mobility.down.sql
├── ...
└── 000023_create_audit_log.up.sql
```

**Migration rules**:

1. **Forward-only in production** — down migrations are for development only.
2. **Additive changes** — new columns use `ALTER TABLE ADD COLUMN` with defaults;
   never drop columns in the same release.
3. **JSONB evolution** — new fields in JSONB columns are additive by nature;
   application code handles missing fields gracefully.
4. **Index changes** — `CREATE INDEX CONCURRENTLY` to avoid blocking reads.
5. **Table renames** — never rename; create new table, migrate data, drop old.

### 9.2 Data Versioning

Every mutable table includes a `version` column (optimistic locking counter)
and `updated_at` timestamp:

```sql
-- Application reads version with data
SELECT version, nssai FROM udm.access_mobility_subscription WHERE supi = $1;

-- Application writes with version check
UPDATE udm.access_mobility_subscription
SET nssai = $new, version = version + 1, updated_at = NOW()
WHERE supi = $1 AND version = $read_version;
```

### 9.3 Data Import from Legacy Systems

For migration from legacy HSS/HLR:

```sql
-- Bulk import using COPY for initial subscriber load
COPY udm.subscribers (supi, gpsi, supi_type, created_at, updated_at)
FROM '/data/export/subscribers.csv' WITH (FORMAT csv, HEADER true);

-- Parallel import strategy:
-- 1. Disable foreign key checks (temporarily drop constraints)
-- 2. COPY data into each table in parallel
-- 3. Re-enable foreign key constraints
-- 4. Run ANALYZE on all tables to update statistics

-- Post-migration validation
SELECT COUNT(*) FROM udm.subscribers;
SELECT COUNT(*) FROM udm.authentication_data;
-- Verify referential integrity
SELECT s.supi FROM udm.subscribers s
LEFT JOIN udm.authentication_data a ON s.supi = a.supi
WHERE a.supi IS NULL;
```

### 9.4 Rolling Schema Updates

Zero-downtime schema changes follow a multi-phase approach:

```
Phase 1: Add new column (nullable, with default)
  ALTER TABLE udm.access_mobility_subscription
  ADD COLUMN new_field JSONB DEFAULT NULL;

Phase 2: Deploy new application version that writes to both old and new columns
  (dual-write period)

Phase 3: Backfill existing rows
  UPDATE udm.access_mobility_subscription
  SET new_field = compute_from_old_data(old_field)
  WHERE new_field IS NULL;
  -- Run in batches of 10,000 with LIMIT/OFFSET to avoid long transactions

Phase 4: Deploy application version that reads from new column only

Phase 5: (Next release) Drop old column if applicable
  ALTER TABLE udm.access_mobility_subscription DROP COLUMN old_field;
```

---

## 10. Storage Estimation

### 10.1 Per-Subscriber Storage

| Table | Rows per Sub | Avg Row Size | Per-Sub Storage |
|-------|-------------|-------------|----------------|
| `subscribers` | 1 | 300 B | 300 B |
| `authentication_data` | 1 | 500 B | 500 B |
| `authentication_status` | 1 | 200 B | 200 B |
| `access_mobility_subscription` | 1-2 | 1.0 KB | 1.5 KB |
| `session_management_subscription` | 2-4 | 500 B | 1.5 KB |
| `smf_selection_data` | 1-2 | 300 B | 450 B |
| `sms_subscription_data` | 1 | 200 B | 200 B |
| `sms_management_data` | 1 | 200 B | 200 B |
| `amf_registrations` | 1-2 | 500 B | 750 B |
| `smf_registrations` | 0-4 | 400 B | 800 B |
| `smsf_registrations` | 0-1 | 300 B | 150 B |
| `ee_subscriptions` | 0-2 | 500 B | 250 B |
| `sdm_subscriptions` | 0-3 | 400 B | 300 B |
| `pp_data` | 0-1 | 400 B | 200 B |
| `network_slice_data` | 1 | 300 B | 300 B |
| `operator_specific_data` | 0-3 | 300 B | 300 B |
| `audit_log` (30-day retention) | ~10 | 200 B | 2.0 KB |
| **Secondary indexes** | — | — | ~2.0 KB |
| **Total per subscriber** | — | — | **~11.6 KB** |

### 10.2 Cluster Storage by Subscriber Count

| Metric | 10M Subscribers | 50M Subscribers | 100M Subscribers |
|--------|----------------|----------------|-----------------|
| **Raw data size** | 116 GB | 580 GB | 1.16 TB |
| **With RF=3 replication** | 348 GB | 1.74 TB | 3.48 TB |
| **With compaction overhead (1.5×)** | 522 GB | 2.61 TB | 5.22 TB |
| **Recommended disk per node (9 nodes)** | 60 GB | 300 GB | 600 GB |
| **Recommended disk per node (27 nodes)** | — | 100 GB | 200 GB |
| **Tablet count (auto-split)** | ~1,500 | ~7,500 | ~15,000 |
| **Estimated total rows (all tables)** | ~150M | ~750M | ~1.5B |

### 10.3 Growth Projections

```
Storage growth rate (per million new subscribers):
  Base data:     ~11.6 GB
  Replicated:    ~34.8 GB (RF=3)
  With overhead: ~52.2 GB

Audit log growth (per day, 100M subscribers, 10% active):
  10M auth events × 200 B = 2 GB/day raw
  With replication: 6 GB/day
  30-day retention: 180 GB

Total with audit (100M subscribers):
  5.22 TB (subscription data) + 0.18 TB (audit) = ~5.4 TB
```

### 10.4 Memory Sizing

| Scale | YugabyteDB Nodes | RAM per Node | Total Cluster RAM | Block Cache (70%) |
|-------|-----------------|-------------|-------------------|-------------------|
| 10M subs | 9 (3 per region) | 32 GB | 288 GB | 201 GB |
| 50M subs | 27 (9 per region) | 64 GB | 1,728 GB | 1,210 GB |
| 100M subs | 54 (18 per region) | 64 GB | 3,456 GB | 2,419 GB |

At 100M subscribers, the block cache (2.4 TB) can hold ~45% of the raw dataset
(5.22 TB) in memory, ensuring that hot subscriber data (recently active
subscribers) is served from cache for sub-millisecond read latency.

---

## References

| Document | Description |
|----------|-------------|
| [architecture.md](architecture.md) | High-Level Architecture (Section 14: Data Model Overview) |
| [service-decomposition.md](service-decomposition.md) | Internal microservice structure and service boundaries |
| [sbi-api-design.md](sbi-api-design.md) | SBI API design patterns and HTTP/2 conventions |
| 3GPP TS 29.505 V18.7.0 | Usage of the UDR Service for Subscription Data |
| 3GPP TS 29.503 | Nudm Services (UEAU, SDM, UECM, EE, PP, MT, SSAU, NIDDAU, RSDS, UEID) |
| 3GPP TS 23.501 | System Architecture for the 5G System |
| [YugabyteDB YSQL Docs](https://docs.yugabyte.com/latest/api/ysql/) | PostgreSQL-compatible distributed SQL reference |

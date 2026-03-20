# UDM Internal Service Decomposition

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |
| **Parent Document** | [architecture.md](architecture.md) |

---

## Table of Contents

1. [Service Decomposition Overview](#1-service-decomposition-overview)
2. [Service Catalog](#2-service-catalog)
3. [Shared Services and Cross-Cutting Components](#3-shared-services-and-cross-cutting-components)
4. [Service Interaction Diagram](#4-service-interaction-diagram)
5. [Go Package Structure](#5-go-package-structure)
6. [Service Communication Patterns](#6-service-communication-patterns)
7. [Responsibility Matrix](#7-responsibility-matrix)
8. [Service Dependencies](#8-service-dependencies)

---

## 1. Service Decomposition Overview

### 1.1 Design Principle

The UDM is decomposed into independent microservices, where **each 3GPP Nudm
service-based interface maps to exactly one internal microservice**. This provides a
clean 1:1 alignment between the 3GPP specification structure (TS 29.503) and the
deployment topology. Each microservice:

- Owns a single Nudm API root (e.g., `/nudm-sdm/v2`, `/nudm-ueau/v1`).
- Is built, containerized, and deployed as an independent Kubernetes `Deployment`.
- Scales horizontally based on its own traffic profile.
- Shares common libraries but has no compile-time dependency on other services.

There is **no separate UDR component**. All services access YugabyteDB directly
through a shared database access layer, as described in the
[architecture document](architecture.md) §7.1.

### 1.2 Decomposition Summary

| # | Microservice | 3GPP Spec | API Root | Path Count | Traffic Tier |
|---|-------------|-----------|----------|------------|-------------|
| 1 | **udm-ueau** | TS29503_Nudm_UEAU.yaml | `/nudm-ueau/v1` | 7 | High |
| 2 | **udm-sdm** | TS29503_Nudm_SDM.yaml | `/nudm-sdm/v2` | 38 | High |
| 3 | **udm-uecm** | TS29503_Nudm_UECM.yaml | `/nudm-uecm/v1` | 17 | High |
| 4 | **udm-ee** | TS29503_Nudm_EE.yaml | `/nudm-ee/v1` | 2 + callbacks | Medium |
| 5 | **udm-pp** | TS29503_Nudm_PP.yaml | `/nudm-pp/v1` | 4 | Medium |
| 6 | **udm-mt** | TS29503_Nudm_MT.yaml | `/nudm-mt/v1` | 2 | Medium |
| 7 | **udm-ssau** | TS29503_Nudm_SSAU.yaml | `/nudm-ssau/v1` | 2 | Low |
| 8 | **udm-niddau** | TS29503_Nudm_NIDDAU.yaml | `/nudm-niddau/v1` | 1 | Low |
| 9 | **udm-rsds** | TS29503_Nudm_RSDS.yaml | `/nudm-rsds/v1` | 1 | Low |
| 10 | **udm-ueid** | TS29503_Nudm_UEID.yaml | `/nudm-ueid/v1` | 1 | Low |

> **Note on SUCI de-concealment**: The UEID service (deconceal endpoint) is logically
> coupled with UEAU authentication flows. In production, `udm-ueau` may invoke
> `udm-ueid` internally for SUCI resolution, or the two may be co-deployed. The UEID
> spec is maintained as a separate service to preserve 1:1 spec alignment.

### 1.3 Traffic Tier Definitions

| Tier | Expected RPS (per 10M subs) | Scaling Policy | Min Replicas |
|------|---------------------------|----------------|-------------|
| **High** | 50,000 – 200,000 | Aggressive HPA, priority scheduling | 6 |
| **Medium** | 5,000 – 30,000 | Standard HPA | 3 |
| **Low** | 500 – 5,000 | Minimum replicas with burst capacity | 2 |

---

## 2. Service Catalog

### 2.1 udm-ueau — UE Authentication

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_UEAU.yaml |
| **API Root** | `/nudm-ueau/v1` |
| **Primary Consumers** | AUSF, HSS/IWF |
| **Traffic Tier** | High |

**Purpose**: Generates authentication vectors for 5G-AKA, EAP-AKA', EPS-AKA, and
GBA authentication procedures. Manages authentication confirmation events and
sequence number (SQN) synchronization. This is the most security-critical service in
the UDM, as it handles long-term subscriber keys (K, OPc) and SUCI de-concealment.

**Endpoints**:

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/{supiOrSuci}/security-information/generate-auth-data` | Generate 5G-AKA or EAP-AKA' authentication vectors |
| GET | `/{supiOrSuci}/security-information-rg` | Retrieve RG (Residential Gateway) security information |
| POST | `/{supi}/auth-events` | Confirm successful authentication (auth event creation) |
| DELETE | `/{supi}/auth-events/{authEventId}` | Delete an authentication event |
| POST | `/{supi}/hss-security-information/{hssAuthType}/generate-av` | Generate auth vectors for 4G/5G interworking (HSS) |
| POST | `/{supi}/gba-security-information/generate-av` | Generate GBA (Generic Bootstrapping Architecture) vectors |
| POST | `/{supiOrSuci}/prose-security-information/generate-av` | Generate ProSe (Proximity Services) auth vectors |

**Key Responsibilities**:
- **Auth Vector Generation**: Computes RAND, AUTN, XRES*, KAUSF using Milenage or
  TUAK algorithms from the subscriber's long-term key (K) and operator variant (OPc).
- **SUCI De-concealment**: When the input identifier is a SUCI, resolves it to a SUPI
  using the HPLMN private key (ECIES Profile A or B) before looking up credentials.
  This reuses the deconceal logic from `udm-ueid`.
- **SQN Management**: Maintains the authentication sequence number per subscriber with
  strict serialization to prevent replay attacks. SQN updates use YugabyteDB
  row-level locking for cross-region consistency.
- **Auth Confirmation**: Records successful authentication events from AUSF, enabling
  audit trails and triggering downstream notifications.
- **HSS Interworking**: Generates EPS-AKA vectors for 4G subscribers roaming into 5G
  coverage via the N26 interface.

**Database Tables Accessed**:
- `auth_credentials` (K, OPc, algorithm_id, SQN) — read + SQN update
- `auth_events` — write on confirmation
- `suci_profiles` (home network public/private keys) — read for SUCI deconceal

---

### 2.2 udm-sdm — Subscriber Data Management

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_SDM.yaml |
| **API Root** | `/nudm-sdm/v2` |
| **Primary Consumers** | AMF, SMF, PCF, NEF, SMSF |
| **Traffic Tier** | High |

**Purpose**: Serves as the primary read interface for all subscriber profile data.
This is the highest-traffic service in the UDM, handling data retrieval for access
and mobility management, session management, slice selection, SMS, location services,
V2X, ProSe, and more. Also manages change notification subscriptions so that
consuming NFs are informed when subscriber data changes.

**Endpoints** (38 paths):

| Category | Method | Path | Operation |
|----------|--------|------|-----------|
| **Profile** | GET | `/{supi}` | Retrieve full subscriber data set |
| **AM Data** | GET | `/{supi}/am-data` | Access and Mobility subscription data |
| | GET | `/{supi}/am-data/ecr-data` | Enhanced Coverage Restriction data |
| | PUT | `/{supi}/am-data/sor-ack` | SoR (Steering of Roaming) acknowledgment |
| | PUT | `/{supi}/am-data/upu-ack` | UE Parameters Update acknowledgment |
| | PUT | `/{supi}/am-data/subscribed-snssais-ack` | Subscribed S-NSSAI acknowledgment |
| | PUT | `/{supi}/am-data/cag-ack` | CAG (Closed Access Group) acknowledgment |
| | PUT | `/{supi}/am-data/update-sor` | Update SoR information |
| **NSSAI** | GET | `/{supi}/nssai` | Network Slice Selection Assistance Info |
| **SM Data** | GET | `/{supi}/sm-data` | Session Management subscription data |
| | GET | `/{supi}/smf-select-data` | SMF selection subscription data |
| **UE Context** | GET | `/{supi}/ue-context-in-amf-data` | UE context in AMF |
| | GET | `/{supi}/ue-context-in-smf-data` | UE context in SMF |
| | GET | `/{supi}/ue-context-in-smsf-data` | UE context in SMSF |
| **SMS** | GET | `/{supi}/sms-data` | SMS subscription data |
| | GET | `/{supi}/sms-mng-data` | SMS management subscription data |
| **LCS** | GET | `/{ueId}/lcs-privacy-data` | LCS privacy subscription data |
| | GET | `/{supi}/lcs-mo-data` | LCS Mobile Originated data |
| | GET | `/{supi}/lcs-bca-data` | LCS Broadcast Assistance data |
| | GET | `/{supi}/lcs-subscription-data` | LCS subscription data |
| | GET | `/{ueId}/rangingsl-privacy-data` | Ranging/SL privacy data |
| **V2X/ProSe** | GET | `/{supi}/v2x-data` | V2X subscription data |
| | GET | `/{supi}/prose-data` | ProSe subscription data |
| | GET | `/{supi}/a2x-data` | A2X subscription data |
| **5MBS** | GET | `/{supi}/5mbs-data` | 5G Multicast/Broadcast data |
| **Other** | GET | `/{supi}/uc-data` | User Consent data |
| | GET | `/{supi}/trace-data` | Trace configuration data |
| | GET | `/{supi}/time-sync-data` | Time Synchronization data |
| | GET | `/{supi}/ranging-slpos-data` | Ranging/SL positioning data |
| **Identity** | GET | `/{ueId}/id-translation-result` | GPSI-to-SUPI translation |
| | GET | `/multiple-identifiers` | Bulk identifier resolution |
| **Shared Data** | GET | `/shared-data` | Retrieve shared subscription data |
| | GET | `/shared-data/{sharedDataId}` | Retrieve specific shared data set |
| | GET | `/group-data/group-identifiers` | Resolve group identifiers |
| **Subscriptions** | POST | `/{ueId}/sdm-subscriptions` | Create SDM change subscription |
| | DELETE | `/{ueId}/sdm-subscriptions/{subscriptionId}` | Delete SDM subscription |
| | PATCH | `/{ueId}/sdm-subscriptions/{subscriptionId}` | Modify SDM subscription |
| | POST | `/shared-data-subscriptions` | Subscribe to shared data changes |
| | DELETE | `/shared-data-subscriptions/{subscriptionId}` | Unsubscribe from shared data |
| | PATCH | `/shared-data-subscriptions/{subscriptionId}` | Modify shared data subscription |

**Key Responsibilities**:
- **Data Retrieval**: Serves subscription data categorized by type (AM, SM, NSSAI,
  SMS, LCS, V2X, ProSe, 5MBS, etc.) from YugabyteDB. Supports filtering by
  PLMN ID, S-NSSAI, and DNN.
- **Change Subscriptions**: NFs subscribe to data change notifications. When
  subscriber data is updated (e.g., via PP service or O&M), SDM triggers callback
  notifications to all active subscribers via the `udm-notify` shared component.
- **Policy Acknowledgments**: Processes SoR, UPU, CAG, and SNSSAI acknowledgments
  from the AMF, updating internal state and potentially triggering further procedures.
- **Shared Data Optimization**: Supports shared data sets for subscribers with
  identical profiles, reducing storage and query overhead.
- **Identity Translation**: Resolves GPSI (e.g., MSISDN) to SUPI and vice versa.

**Database Tables Accessed**:
- `subscription_data` — read (all data type categories)
- `sdm_subscriptions` — read/write for subscription management
- `shared_subscription_data` — read for shared data queries
- `identity_mapping` — read for GPSI/SUPI translation

**Caching Strategy**: AM data and NSSAI are cached aggressively in Redis (TTL 60s)
due to high read frequency and low change rate.

---

### 2.3 udm-uecm — UE Context Management

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_UECM.yaml |
| **API Root** | `/nudm-uecm/v1` |
| **Primary Consumers** | AMF, SMF, SMSF, NWDAF |
| **Traffic Tier** | High |

**Purpose**: Tracks serving NF registrations for each subscriber — which AMF, SMF,
and SMSF are currently serving a given UE. This is the write-heavy counterpart to
SDM's read-heavy profile queries, as registration state changes with every attach,
handover, and session establishment.

**Endpoints** (17 paths):

| Method | Path | Operation |
|--------|------|-----------|
| GET | `/{ueId}/registrations` | Get all current NF registrations |
| PUT | `/{ueId}/registrations/amf-3gpp-access` | Register/update AMF for 3GPP access |
| PATCH | `/{ueId}/registrations/amf-3gpp-access` | Partial update AMF registration |
| PUT | `/{ueId}/registrations/amf-non-3gpp-access` | Register AMF for non-3GPP access |
| PATCH | `/{ueId}/registrations/amf-non-3gpp-access` | Partial update non-3GPP AMF |
| POST | `/{ueId}/registrations/amf-3gpp-access/dereg-amf` | AMF-initiated deregistration |
| POST | `/{ueId}/registrations/amf-3gpp-access/pei-update` | Update PEI (device identity) |
| POST | `/{ueId}/registrations/amf-3gpp-access/roaming-info-update` | Update roaming information |
| PUT | `/{ueId}/registrations/smf-registrations/{pduSessionId}` | Register SMF for a PDU session |
| DELETE | `/{ueId}/registrations/smf-registrations/{pduSessionId}` | Deregister SMF |
| GET | `/{ueId}/registrations/smf-registrations` | Get all SMF registrations |
| PUT | `/{ueId}/registrations/smsf-3gpp-access` | Register SMSF for 3GPP access |
| DELETE | `/{ueId}/registrations/smsf-3gpp-access` | Deregister SMSF (3GPP) |
| PUT | `/{ueId}/registrations/smsf-non-3gpp-access` | Register SMSF for non-3GPP access |
| DELETE | `/{ueId}/registrations/smsf-non-3gpp-access` | Deregister SMSF (non-3GPP) |
| PUT | `/{ueId}/registrations/ip-sm-gw` | Register IP-SM-GW |
| POST | `/restore-pcscf` | Restore P-CSCF for IMS |
| GET | `/{ueId}/registrations/location` | Retrieve UE location registration |
| PUT | `/{ueId}/registrations/nwdaf-registrations/{nwdafRegistrationId}` | Register NWDAF |
| DELETE | `/{ueId}/registrations/nwdaf-registrations/{nwdafRegistrationId}` | Deregister NWDAF |
| POST | `/{ueId}/registrations/send-routing-info-sm` | Get SMS routing info |
| POST | `/{ueId}/registrations/trigger-auth` | Trigger re-authentication |

**Key Responsibilities**:
- **AMF Registration**: Stores the serving AMF instance, GUAMI, and access type for
  each subscriber. Enforces mutual exclusion — only one AMF per access type.
- **SMF Registration**: Tracks PDU session context per session ID, including DNN,
  S-NSSAI, and SMF instance.
- **SMSF Registration**: Manages SMS-over-NAS serving function registrations.
- **Deregistration**: Handles explicit and implicit deregistration, cleaning up stale
  NF registrations and triggering notifications to affected NFs.
- **PEI Updates**: Records the Permanent Equipment Identifier (IMEI/IMEISV) reported
  by the AMF during registration.
- **SMS Routing**: Provides routing information for mobile-terminated SMS delivery.

**Database Tables Accessed**:
- `amf_registrations` — read/write
- `smf_registrations` — read/write
- `smsf_registrations` — read/write
- `nwdaf_registrations` — read/write

**Consistency Requirement**: Registration updates require strong consistency (Raft
leader writes) to prevent split-brain scenarios where two regions disagree on which
AMF is serving a subscriber.

---

### 2.4 udm-ee — Event Exposure

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_EE.yaml |
| **API Root** | `/nudm-ee/v1` |
| **Primary Consumers** | NEF, NWDAF, AMF |
| **Traffic Tier** | Medium |

**Purpose**: Manages event subscriptions from consuming NFs. Subscribers can request
notifications for events such as UE reachability, location changes, connectivity
state, loss of connectivity, communication failure, and UE registration state.

**Endpoints** (2 resource paths + callback mechanism):

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/{ueIdentity}/ee-subscriptions` | Create event exposure subscription |
| DELETE | `/{ueIdentity}/ee-subscriptions/{subscriptionId}` | Delete subscription |
| PATCH | `/{ueIdentity}/ee-subscriptions/{subscriptionId}` | Modify subscription |

**Callback** (asynchronous, outbound):

| Direction | Path | Trigger |
|-----------|------|---------|
| UDM → NF Consumer | `{callbackReference}` | Event condition met (e.g., UE becomes reachable) |

**Supported Event Types**:
- UE reachability for data / SMS
- Location reporting (cell-level, TA-level)
- Loss of connectivity
- Communication failure
- UE registration state changes
- Connectivity state (IDLE / CONNECTED)
- PDN connectivity status
- Roaming status changes
- Change of SUPI-PEI association

**Key Responsibilities**:
- **Subscription Management**: Creates, modifies, and deletes event subscriptions
  with configurable expiry, event filters, and callback URIs.
- **Event Correlation**: Monitors registration and context changes (from UECM) to
  detect when subscribed event conditions are met.
- **Callback Dispatch**: Delegates outbound notifications to the `udm-notify` shared
  component for reliable delivery with retry logic.

**Database Tables Accessed**:
- `ee_subscriptions` — read/write
- `ee_event_reports` — write (audit log of delivered events)

---

### 2.5 udm-pp — Parameter Provisioning

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_PP.yaml |
| **API Root** | `/nudm-pp/v1` |
| **Primary Consumers** | NEF, AF (via NEF) |
| **Traffic Tier** | Medium |

**Purpose**: Enables external provisioning of subscriber parameters and group
management. Used by NEF/AF to update per-UE parameters, manage 5G VN (Virtual
Network) groups, and manage MBS (Multicast/Broadcast Service) group membership.

**Endpoints** (4 resource paths, multiple methods per path):

| Method | Path | Operation |
|--------|------|-----------|
| PATCH | `/{ueId}/pp-data` | Update provisioned parameters for a UE |
| GET | `/{ueId}/pp-data` | Retrieve provisioned parameters |
| PUT | `/5g-vn-groups/{extGroupId}` | Create/update 5G VN group |
| DELETE | `/5g-vn-groups/{extGroupId}` | Delete 5G VN group |
| PATCH | `/5g-vn-groups/{extGroupId}` | Modify 5G VN group |
| GET | `/5g-vn-groups/{extGroupId}` | Retrieve 5G VN group |
| PUT | `/{ueId}/pp-data-store/{afInstanceId}` | Store AF-specific PP data |
| DELETE | `/{ueId}/pp-data-store/{afInstanceId}` | Delete AF-specific PP data |
| GET | `/{ueId}/pp-data-store/{afInstanceId}` | Retrieve AF-specific PP data |
| PUT | `/mbs-group-membership/{extGroupId}` | Create/update MBS group membership |
| DELETE | `/mbs-group-membership/{extGroupId}` | Delete MBS group membership |
| GET | `/mbs-group-membership/{extGroupId}` | Retrieve MBS group membership |

**Key Responsibilities**:
- **Per-UE Provisioning**: Updates subscriber-specific parameters such as expected UE
  behavior, communication duration, periodic communication indicators, and scheduled
  communication time.
- **5G VN Group Management**: Creates and maintains virtual network group definitions
  including member lists, DNN, S-NSSAI, and PDU session types for group communication.
- **MBS Group Membership**: Manages multicast/broadcast service group membership for
  5MBS scenarios.
- **Change Propagation**: After parameter updates, triggers SDM change notifications
  to NFs that have active subscriptions for the affected data.

**Database Tables Accessed**:
- `pp_data` — read/write
- `vn_groups` — read/write
- `vn_group_members` — read/write
- `mbs_group_membership` — read/write

---

### 2.6 udm-mt — Mobile Terminated

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_MT.yaml |
| **API Root** | `/nudm-mt/v1` |
| **Primary Consumers** | SMSF, AMF |
| **Traffic Tier** | Medium |

**Purpose**: Provides UE information for mobile-terminated service delivery —
specifically querying the current location and reachability state of a subscriber.
Used when an NF needs to determine where to route a mobile-terminated request (e.g.,
SMS, IMS call) before delivery.

**Endpoints** (2 paths):

| Method | Path | Operation |
|--------|------|-----------|
| GET | `/{supi}` | Query UE info (serving AMF, user state, reachability) |
| POST | `/{supi}/loc-info/provide-loc-info` | Provide location information for a UE |

**Key Responsibilities**:
- **UE Info Query**: Returns the current serving AMF, user state (registered,
  deregistered, connected, idle), and last known location. Internally reads from
  UECM registration data.
- **Location Provisioning**: Accepts location updates from the serving AMF for use in
  subsequent MT routing decisions.

**Database Tables Accessed**:
- `amf_registrations` — read (serving AMF, user state)
- `ue_location` — read/write

---

### 2.7 udm-ssau — Service-Specific Authorization

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_SSAU.yaml |
| **API Root** | `/nudm-ssau/v1` |
| **Primary Consumers** | NEF, AF (via NEF) |
| **Traffic Tier** | Low |

**Purpose**: Authorizes service-specific requests on a per-subscriber basis. Checks
whether a given UE is authorized for a specific service type (e.g., URSP policy
delivery, QoS-specific services, ProSe, V2X).

**Endpoints** (2 paths):

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/{ueIdentity}/{serviceType}/authorize` | Authorize UE for a service type |
| POST | `/{ueIdentity}/{serviceType}/remove` | Remove service authorization |

**Key Responsibilities**:
- **Authorization Check**: Evaluates whether the subscriber's profile includes
  entitlement for the requested service type. Returns authorization result with
  optional conditions and constraints.
- **Authorization Removal**: Revokes a previously granted service-specific
  authorization.

**Database Tables Accessed**:
- `subscription_data` (service-specific entitlements) — read
- `ssau_authorizations` — read/write

---

### 2.8 udm-niddau — NIDD Authorization

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_NIDDAU.yaml |
| **API Root** | `/nudm-niddau/v1` |
| **Primary Consumers** | NEF |
| **Traffic Tier** | Low |

**Purpose**: Authorizes Non-IP Data Delivery (NIDD) configurations. NIDD allows IoT
devices to exchange data with application servers without IP stack overhead, using the
NEF as a relay. The UDM validates that the subscriber is authorized for NIDD and
returns the authorized configuration.

**Endpoints** (1 path):

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/{ueIdentity}/authorize` | Authorize NIDD configuration |

**Key Responsibilities**:
- **NIDD Authorization**: Validates that the subscriber's profile permits NIDD for the
  requested DNN and S-NSSAI. Returns the authorized NIDD configuration including
  allowed data rate and reliability requirements.

**Database Tables Accessed**:
- `subscription_data` (NIDD authorization data) — read

---

### 2.9 udm-rsds — Report SMS Delivery Status

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_RSDS.yaml |
| **API Root** | `/nudm-rsds/v1` |
| **Primary Consumers** | SMSF |
| **Traffic Tier** | Low |

**Purpose**: Receives SMS delivery status reports from the SMSF. When an SMS is
delivered (or delivery fails), the SMSF reports the outcome to the UDM via this
service, allowing the UDM to update internal state and notify interested parties.

**Endpoints** (1 path):

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/{ueIdentity}/sm-delivery-status` | Report SMS delivery status |

**Key Responsibilities**:
- **Delivery Status Recording**: Records the delivery outcome (success, failure,
  pending) for mobile-terminated SMS messages.
- **Status Propagation**: If other NFs have subscribed to SMS delivery events via
  the EE service, triggers callback notifications.

**Database Tables Accessed**:
- `sms_delivery_status` — write

---

### 2.10 udm-ueid — UE Identification

| Property | Value |
|----------|-------|
| **3GPP Spec** | TS29503_Nudm_UEID.yaml |
| **API Root** | `/nudm-ueid/v1` |
| **Primary Consumers** | AUSF (via udm-ueau) |
| **Traffic Tier** | Low |

**Purpose**: Provides SUCI (Subscription Concealed Identifier) de-concealment — the
process of decrypting a SUCI to recover the underlying SUPI. This is a cryptographic
operation using the HPLMN's private key (ECIES Profile A or B per TS 33.501 §6.12).

**Endpoints** (1 path):

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/deconceal` | De-conceal SUCI to SUPI |

**Key Responsibilities**:
- **SUCI De-concealment**: Decrypts the SUCI using the home network's ECIES private
  key. Supports both Profile A (Curve25519/X25519) and Profile B (secp256r1).
- **Key Management**: Reads the active HPLMN key pair from secure storage. Supports
  key rotation with multiple active key identifiers.

**Database Tables Accessed**:
- `suci_profiles` (HPLMN public/private key pairs, key IDs) — read

> **Deployment Note**: Due to the low request volume and tight coupling with
> authentication flows, `udm-ueid` may be co-deployed with `udm-ueau` as a single
> binary using build tags or a shared process. The service boundary is preserved at
> the API level regardless of deployment model.

---

## 3. Shared Services and Cross-Cutting Components

The following shared components are consumed by all (or most) microservices via Go
package imports. They are **not** deployed as independent services — they are
compiled into each microservice binary as shared libraries.

### 3.1 Component Overview

```
┌────────────────────────────────────────────────────────────────────────┐
│                    Shared Libraries (internal/)                        │
│                                                                        │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐                 │
│  │  udm-common  │  │   udm-db     │  │  udm-cache   │                 │
│  │              │  │              │  │              │                 │
│  │ SUPI/GPSI    │  │ pgx pool     │  │ In-memory    │                 │
│  │ SUCI parsing │  │ Query build  │  │ Redis client │                 │
│  │ Error codes  │  │ Transactions │  │ TTL mgmt     │                 │
│  │ SBI codec    │  │ Migrations   │  │ Invalidation │                 │
│  │ Logging      │  │ Health check │  │              │                 │
│  │ Config       │  │              │  │              │                 │
│  └──────────────┘  └──────────────┘  └──────────────┘                 │
│                                                                        │
│  ┌──────────────┐  ┌──────────────┐                                   │
│  │ udm-notify   │  │  udm-nrf     │                                   │
│  │              │  │              │                                   │
│  │ Callback     │  │ NF register  │                                   │
│  │ Retry logic  │  │ Discovery    │                                   │
│  │ Circuit brkr │  │ Heartbeat    │                                   │
│  │ Webhook mgmt │  │ OAuth2 token │                                   │
│  └──────────────┘  └──────────────┘                                   │
└────────────────────────────────────────────────────────────────────────┘
```

### 3.2 udm-common

**Package**: `internal/common`

Shared utilities used by every microservice:

| Sub-package | Responsibility |
|-------------|---------------|
| `common/identifiers` | SUPI format validation (`imsi-<MCC><MNC><MSIN>`), GPSI parsing (`msisdn-<CC><NDC><SN>`), SUCI structure parsing (scheme ID, HN key ID, protection scheme, cipher text) |
| `common/errors` | 3GPP problem details (RFC 7807) with standard `ProblemDetails` struct, cause codes per TS 29.503, HTTP status mapping |
| `common/sbi` | HTTP/2 client/server helpers, 3GPP custom header handling (`3gpp-Sbi-Target-apiRoot`, `3gpp-Sbi-Callback`, `3gpp-Sbi-Oci`), content-type negotiation |
| `common/logging` | Structured JSON logger (zerolog or slog), correlation ID propagation, 3GPP trace ID support |
| `common/config` | Environment-based configuration loading, feature flags, runtime config reload via ConfigMap watch |
| `common/health` | Kubernetes readiness/liveness probe handlers, graceful shutdown orchestration |
| `common/telemetry` | OpenTelemetry SDK initialization, span creation, metric registration (RED metrics) |

### 3.3 udm-db

**Package**: `internal/db`

Database access layer for YugabyteDB (YSQL — PostgreSQL wire protocol):

| Component | Responsibility |
|-----------|---------------|
| **Connection Pool** | pgx connection pool with configurable min/max connections, idle timeout, health checks, and connection lifetime. Geo-aware connection routing to prefer local YugabyteDB tablet servers. |
| **Query Builder** | Type-safe SQL query construction for common access patterns (SUPI lookups, registration upserts, subscription CRUD). Avoids raw SQL string concatenation. |
| **Transaction Manager** | Wraps YugabyteDB transactions with automatic retry on serialization conflicts (`40001`). Supports read-only and read-write transaction modes. |
| **Migration Runner** | Schema migration using `golang-migrate` or embedded SQL files. Runs as a Kubernetes Job during deployments. |
| **Health Check** | Periodic connection validation, latency probing, and tablet server reachability checks exposed via the health endpoint. |

### 3.4 udm-cache

**Package**: `internal/cache`

Two-tier caching layer for read-heavy data:

| Tier | Implementation | Use Case |
|------|---------------|----------|
| **L1 (In-Memory)** | `sync.Map` or Ristretto with LRU eviction | Per-pod hot data (SUCI key cache, frequent SUPI lookups). TTL: 10–30s. |
| **L2 (Redis)** | Redis Cluster (regional) via `go-redis` | Shared subscriber profile cache across pods. TTL: 30–120s. |

**Cache Invalidation Strategy**:
- Write-through invalidation: when PP or UECM writes data, the writing service
  issues a cache `DEL` for the affected SUPI key.
- TTL-based expiry as a safety net for missed invalidations.
- No cross-region cache replication — each region maintains an independent Redis
  cache warmed by local read traffic.

### 3.5 udm-notify

**Package**: `internal/notify`

Asynchronous notification and callback engine:

| Feature | Description |
|---------|-------------|
| **Callback Dispatch** | HTTP/2 POST to NF consumer callback URIs with 3GPP-compliant notification payloads. |
| **Retry Policy** | Exponential backoff with jitter (initial: 1s, max: 30s, max retries: 5). Configurable per notification type. |
| **Circuit Breaker** | Per-destination circuit breaker (closed → open on 5 consecutive failures, half-open probe every 30s). Prevents cascading failures to unreachable NFs. |
| **Batch Delivery** | Groups multiple notifications for the same callback URI into a single HTTP request where the spec allows (e.g., SDM change notifications). |
| **Dead Letter Queue** | Failed notifications after max retries are written to a DLQ table in YugabyteDB for manual inspection and replay. |

### 3.6 udm-nrf

**Package**: `internal/nrf`

NRF client for service registration and discovery per TS 29.510:

| Feature | Description |
|---------|-------------|
| **NF Registration** | Registers the UDM NF profile with the NRF on startup, including supported services, PLMN IDs, and capacity information. |
| **Heartbeat** | Periodic PATCH to NRF to maintain registration (configurable interval, default: 30s). |
| **NF Discovery** | Discovers other NF instances (e.g., AUSF, AMF) by querying the NRF. Results are cached locally. |
| **OAuth2 Token** | Obtains and caches OAuth2 access tokens from the NRF for authenticating outbound SBI requests. |
| **Deregistration** | Graceful NF deregistration on pod shutdown. |

---

## 4. Service Interaction Diagram

### 4.1 External NF-to-UDM Interactions

```
                          5G Core NF Consumers
    ┌──────────────────────────────────────────────────────┐
    │                                                      │
    │   ┌──────┐   ┌──────┐   ┌──────┐   ┌──────┐        │
    │   │ AUSF │   │ AMF  │   │ SMF  │   │ SMSF │        │
    │   └──┬───┘   └──┬───┘   └──┬───┘   └──┬───┘        │
    │      │          │          │          │              │
    │   ┌──┴──┐   ┌──┴──┐   ┌──┴──┐   ┌──┴──┐            │
    │   │ NEF │   │ PCF │   │NWDAF│   │ AF  │            │
    │   └──┬──┘   └──┬──┘   └──┬──┘   └──┬──┘            │
    └──────┼─────────┼─────────┼─────────┼────────────────┘
           │         │         │         │
           ▼         ▼         ▼         ▼
    ═══════════════════════════════════════════════════
              Istio Gateway / Service Mesh (mTLS)
    ═══════════════════════════════════════════════════
           │         │         │         │
    ┌──────┼─────────┼─────────┼─────────┼────────────────┐
    │      ▼         ▼         ▼         ▼                │
    │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐       │
    │  │  ueau  │ │  sdm   │ │  uecm  │ │   ee   │       │
    │  └────────┘ └────────┘ └────────┘ └────────┘       │
    │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐       │
    │  │   pp   │ │   mt   │ │  ssau  │ │ niddau │       │
    │  └────────┘ └────────┘ └────────┘ └────────┘       │
    │  ┌────────┐ ┌────────┐                              │
    │  │  rsds  │ │  ueid  │      UDM Service Layer       │
    │  └────────┘ └────────┘                              │
    └─────────────────────────────────────────────────────┘
```

### 4.2 Internal Service-to-Service Communication

```
┌──────────────────────────────────────────────────────────────────────┐
│                        UDM Internal Architecture                     │
│                                                                      │
│  ┌────────┐    SUCI deconceal     ┌────────┐                        │
│  │  ueau  │ ──────────────────►   │  ueid  │                        │
│  │        │                       │        │                        │
│  └───┬────┘                       └────────┘                        │
│      │ auth event notification                                       │
│      ▼                                                               │
│  ┌────────┐    registration data  ┌────────┐                        │
│  │   ee   │ ◄──────────────────── │  uecm  │                        │
│  │        │    (event triggers)   │        │                        │
│  └───┬────┘                       └───┬────┘                        │
│      │ callbacks                      │                              │
│      ▼                                │ context reads                 │
│  ┌────────┐                       ┌───▼────┐                        │
│  │ notify │                       │  sdm   │                        │
│  │(shared)│ ◄──────────────────── │        │                        │
│  └────────┘    change notif       └───┬────┘                        │
│                                       │                              │
│      ┌────────────────────────────────┤                              │
│      │ data change triggers           │ profile reads                │
│      ▼                                ▼                              │
│  ┌────────┐                       ┌────────┐                        │
│  │   pp   │ ────────────────────► │   db   │ ◄── all services       │
│  │        │    writes to DB       │(shared)│                        │
│  └────────┘                       └───┬────┘                        │
│                                       │                              │
│                                       ▼                              │
│                               ┌──────────────┐                      │
│                               │  YugabyteDB  │                      │
│                               └──────────────┘                      │
└──────────────────────────────────────────────────────────────────────┘
```

### 4.3 NF Consumer → UDM Service Mapping

```
  AUSF ─────────────► udm-ueau  (auth vectors)
       ─────────────► udm-ueid  (SUCI deconceal, via ueau)

  AMF  ─────────────► udm-sdm   (AM data, NSSAI, SM select)
       ─────────────► udm-uecm  (AMF registration)
       ─────────────► udm-mt    (UE reachability)

  SMF  ─────────────► udm-sdm   (SM data, DNN policies)
       ─────────────► udm-uecm  (SMF registration)

  SMSF ─────────────► udm-sdm   (SMS subscription data)
       ─────────────► udm-uecm  (SMSF registration)
       ─────────────► udm-mt    (UE reachability for SMS)
       ─────────────► udm-rsds  (delivery status)

  NEF  ─────────────► udm-sdm   (subscriber data exposure)
       ─────────────► udm-ee    (event subscriptions)
       ─────────────► udm-pp    (parameter provisioning)
       ─────────────► udm-ssau  (service authorization)
       ─────────────► udm-niddau(NIDD authorization)

  PCF  ─────────────► udm-sdm   (policy subscription data)

  NWDAF ────────────► udm-ee    (analytics event subs)
        ────────────► udm-uecm  (NWDAF registration)
```

---

## 5. Go Package Structure

```
dsub/
├── cmd/                                  # Service entry points
│   ├── udm-ueau/
│   │   └── main.go                       # UE Authentication service
│   ├── udm-sdm/
│   │   └── main.go                       # Subscriber Data Management service
│   ├── udm-uecm/
│   │   └── main.go                       # UE Context Management service
│   ├── udm-ee/
│   │   └── main.go                       # Event Exposure service
│   ├── udm-pp/
│   │   └── main.go                       # Parameter Provisioning service
│   ├── udm-mt/
│   │   └── main.go                       # Mobile Terminated service
│   ├── udm-ssau/
│   │   └── main.go                       # Service-Specific Authorization service
│   ├── udm-niddau/
│   │   └── main.go                       # NIDD Authorization service
│   ├── udm-rsds/
│   │   └── main.go                       # Report SMS Delivery Status service
│   └── udm-ueid/
│       └── main.go                       # UE Identification service
│
├── internal/                             # Private application code
│   ├── ueau/                             # UEAU business logic
│   │   ├── handler.go                    #   HTTP handlers
│   │   ├── service.go                    #   Business logic (auth vector gen)
│   │   ├── milenage.go                   #   Milenage algorithm implementation
│   │   ├── tuak.go                       #   TUAK algorithm implementation
│   │   ├── sqn.go                        #   SQN management
│   │   └── handler_test.go              #   Unit tests
│   ├── sdm/                              # SDM business logic
│   │   ├── handler.go                    #   HTTP handlers (38 endpoints)
│   │   ├── service.go                    #   Data retrieval logic
│   │   ├── subscription.go              #   SDM subscription management
│   │   ├── shared_data.go               #   Shared data operations
│   │   └── handler_test.go              #   Unit tests
│   ├── uecm/                             # UECM business logic
│   │   ├── handler.go                    #   HTTP handlers
│   │   ├── service.go                    #   Registration management
│   │   ├── amf_registration.go          #   AMF-specific logic
│   │   ├── smf_registration.go          #   SMF-specific logic
│   │   └── handler_test.go              #   Unit tests
│   ├── ee/                               # EE business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── event_correlation.go         #   Event detection logic
│   │   └── handler_test.go
│   ├── pp/                               # PP business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── vn_group.go                  #   5G VN group management
│   │   └── handler_test.go
│   ├── mt/                               # MT business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── handler_test.go
│   ├── ssau/                             # SSAU business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── handler_test.go
│   ├── niddau/                           # NIDDAU business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── handler_test.go
│   ├── rsds/                             # RSDS business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   └── handler_test.go
│   ├── ueid/                             # UEID business logic
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── ecies.go                     #   ECIES decryption (Profile A/B)
│   │   └── handler_test.go
│   │
│   ├── common/                           # Shared utilities
│   │   ├── identifiers/                 #   SUPI/GPSI/SUCI handling
│   │   │   ├── supi.go
│   │   │   ├── gpsi.go
│   │   │   ├── suci.go
│   │   │   └── identifiers_test.go
│   │   ├── errors/                      #   3GPP error handling
│   │   │   ├── problem_details.go
│   │   │   └── causes.go
│   │   ├── sbi/                         #   SBI protocol helpers
│   │   │   ├── headers.go
│   │   │   ├── client.go
│   │   │   └── server.go
│   │   ├── logging/                     #   Structured logging
│   │   │   └── logger.go
│   │   ├── config/                      #   Configuration
│   │   │   └── config.go
│   │   ├── health/                      #   Health probes
│   │   │   └── health.go
│   │   └── telemetry/                   #   OpenTelemetry
│   │       └── otel.go
│   │
│   ├── db/                               # Database access layer
│   │   ├── pool.go                      #   Connection pool management
│   │   ├── queries.go                   #   Common query patterns
│   │   ├── tx.go                        #   Transaction manager
│   │   ├── migrate.go                   #   Schema migration runner
│   │   └── health.go                    #   DB health check
│   │
│   ├── cache/                            # Caching layer
│   │   ├── memory.go                    #   L1 in-memory cache
│   │   ├── redis.go                     #   L2 Redis cache
│   │   └── cache.go                     #   Unified cache interface
│   │
│   └── notify/                           # Notification engine
│       ├── dispatcher.go                #   Callback dispatch
│       ├── retry.go                     #   Retry with backoff
│       ├── circuit_breaker.go           #   Circuit breaker
│       └── dlq.go                       #   Dead letter queue
│
├── pkg/                                  # Public packages (importable by external tools)
│   ├── models/                           # Generated OpenAPI models
│   │   ├── sdm_models.go               #   SDM request/response types
│   │   ├── uecm_models.go              #   UECM request/response types
│   │   ├── ueau_models.go              #   UEAU request/response types
│   │   ├── ee_models.go                #   EE request/response types
│   │   ├── common_models.go            #   Shared types (ProblemDetails, etc.)
│   │   └── subscription_data.go        #   TS 29.505 data model types
│   └── client/                           # Generated Nudm client SDK
│       ├── sdm_client.go               #   SDM API client
│       ├── uecm_client.go              #   UECM API client
│       ├── ueau_client.go              #   UEAU API client
│       └── ...                          #   Other service clients
│
├── api/                                  # OpenAPI specifications
│   └── 3gpp/                            #   3GPP YAML specs (source of truth)
│       ├── TS29503_Nudm_SDM.yaml
│       ├── TS29503_Nudm_UECM.yaml
│       └── ...
│
├── deployments/                          # Kubernetes manifests
│   ├── base/                            #   Kustomize base
│   │   ├── udm-ueau/
│   │   ├── udm-sdm/
│   │   └── ...
│   └── overlays/                        #   Environment overlays
│       ├── dev/
│       ├── staging/
│       └── production/
│
├── migrations/                           # Database schema migrations
│   ├── 001_auth_credentials.up.sql
│   ├── 001_auth_credentials.down.sql
│   ├── 002_subscription_data.up.sql
│   └── ...
│
├── docs/                                 # Documentation
│   ├── architecture.md
│   ├── service-decomposition.md         #   (this document)
│   └── 3gpp/                            #   3GPP reference specs
│
├── go.mod
├── go.sum
└── Makefile
```

---

## 6. Service Communication Patterns

### 6.1 Synchronous Communication (HTTP/2)

All Nudm service-based interface calls follow the **synchronous request-response**
pattern mandated by TS 29.500:

```
NF Consumer                    UDM Service
    │                              │
    │  HTTP/2 POST/GET/PUT/PATCH   │
    │ ────────────────────────────► │
    │                              │
    │  HTTP/2 200/201/204 + JSON   │
    │ ◄──────────────────────────── │
    │                              │
```

**Protocol Details**:

| Aspect | Specification |
|--------|--------------|
| **Transport** | HTTP/2 over TLS 1.3 (mandatory per TS 33.501) |
| **Serialization** | JSON (RFC 8259) with `Content-Type: application/json` |
| **Authentication** | OAuth2 bearer token in `Authorization` header |
| **Timeouts** | Client-side: 3s (read), 1s (connect). Server-side: 5s request deadline. |
| **Retry** | Client-side retry with idempotency key for safe methods (GET, PUT, DELETE) |
| **Error Format** | RFC 7807 `ProblemDetails` with 3GPP `cause` extension per TS 29.500 |

### 6.2 Asynchronous Communication (Callbacks)

Event-driven notifications use the **callback pattern** specified in TS 29.500 §6.2:

```
NF Consumer                    UDM (EE/SDM)                 UDM (notify)
    │                              │                              │
    │  POST /ee-subscriptions      │                              │
    │  { callbackReference: "..." }│                              │
    │ ────────────────────────────► │                              │
    │                              │                              │
    │  201 Created                 │                              │
    │ ◄──────────────────────────── │                              │
    │                              │                              │
    │          ... time passes, event occurs ...                   │
    │                              │                              │
    │                              │  Dispatch notification       │
    │                              │ ────────────────────────────► │
    │                              │                              │
    │  POST {callbackReference}    │                              │
    │ ◄──────────────────────────────────────────────────────────── │
    │                              │                              │
    │  204 No Content              │                              │
    │ ─────────────────────────────────────────────────────────────►│
    │                              │                              │
```

**Callback Scenarios**:

| Source Service | Event Type | Triggered By |
|---------------|-----------|-------------|
| **udm-ee** | UE reachability, location, connectivity | UECM registration changes |
| **udm-sdm** | Subscriber data change | PP parameter updates, O&M provisioning |
| **udm-sdm** | Shared data change | Shared data set updates |
| **udm-rsds** | SMS delivery status | SMSF delivery reports |

### 6.3 Internal Service-to-Service Calls

Internal communication between UDM microservices uses the same HTTP/2 + JSON
protocol as external SBI calls, routed through the Kubernetes service mesh:

```
udm-ueau                       udm-ueid
    │                              │
    │  POST /nudm-ueid/v1/deconceal│
    │  (in-cluster, via K8s Service)│
    │ ────────────────────────────► │
    │                              │
    │  200 OK { supi: "..." }      │
    │ ◄──────────────────────────── │
    │                              │
```

**Internal Call Map**:

| Caller | Callee | Purpose |
|--------|--------|---------|
| udm-ueau | udm-ueid | SUCI de-concealment during auth vector generation |
| udm-ee | udm-uecm | Query registration state for event correlation |
| udm-ee | udm-sdm | Query subscriber data for event filtering |
| udm-mt | udm-uecm | Query serving AMF for MT routing |
| udm-pp | udm-sdm | Trigger change notifications after provisioning |
| udm-pp | udm-notify | Dispatch change callbacks to subscribers |
| udm-ee | udm-notify | Dispatch event callbacks to subscribers |
| udm-sdm | udm-notify | Dispatch change callbacks to subscribers |

### 6.4 Event-Driven Patterns

For loose coupling between services, an internal event bus pattern can be used for
non-latency-critical interactions:

```
┌──────────┐                    ┌──────────────┐                ┌──────────┐
│  udm-pp  │  ──publish──►      │  Event Bus   │  ──consume──►  │  udm-sdm │
│          │  "data.changed"    │  (optional)  │               │  (notify) │
└──────────┘                    └──────────────┘                └──────────┘
                                       │
                                       │  ──consume──►  ┌──────────┐
                                       │                │  udm-ee  │
                                       │                │  (check) │
                                       │                └──────────┘
```

**Implementation Options**:

| Option | Implementation | Trade-off |
|--------|---------------|-----------|
| **Direct HTTP** | Service-to-service REST calls | Simple, but tight coupling |
| **YugabyteDB LISTEN/NOTIFY** | PostgreSQL notification channels | Leverages existing infra, limited throughput |
| **Redis Pub/Sub** | Redis channel subscriptions | Low latency, at-most-once delivery |
| **Outbox Pattern** | DB writes + polling reader | Guaranteed delivery, higher latency |

The default implementation uses **direct HTTP calls** for simplicity, with the outbox
pattern available for scenarios requiring guaranteed delivery (e.g., EE event
notifications that must not be lost).

---

## 7. Responsibility Matrix

### 7.1 3GPP Operations by Service

| 3GPP Operation | Service | HTTP Method | Description |
|---------------|---------|-------------|-------------|
| **GenerateAuthData** | udm-ueau | POST | Generate 5G-HE-AKA / EAP-AKA' vectors |
| **GenerateHSSAV** | udm-ueau | POST | Generate HSS auth vectors (4G interwork) |
| **GenerateGBAAV** | udm-ueau | POST | Generate GBA auth vectors |
| **GenerateProSeAV** | udm-ueau | POST | Generate ProSe auth vectors |
| **ConfirmAuth** | udm-ueau | POST | Confirm authentication success |
| **DeleteAuth** | udm-ueau | DELETE | Delete auth event |
| **GetRGAuthData** | udm-ueau | GET | Residential Gateway auth info |
| **Deconceal** | udm-ueid | POST | SUCI → SUPI de-concealment |
| **GetSubscriptionData** | udm-sdm | GET | Retrieve AM/SM/NSSAI/SMS data |
| **GetSharedData** | udm-sdm | GET | Retrieve shared data sets |
| **GetGroupIdentifiers** | udm-sdm | GET | Resolve group identifiers |
| **GetMultipleIdentifiers** | udm-sdm | GET | Bulk identifier resolution |
| **Subscribe (SDM)** | udm-sdm | POST | Create data change subscription |
| **Unsubscribe (SDM)** | udm-sdm | DELETE | Delete data change subscription |
| **ModifySubscription (SDM)** | udm-sdm | PATCH | Modify existing subscription |
| **SoR/UPU/CAG/SNSSAI Ack** | udm-sdm | PUT | Policy acknowledgments |
| **UpdateSoR** | udm-sdm | PUT | Update SoR information |
| **TranslateIdentity** | udm-sdm | GET | GPSI ↔ SUPI translation |
| **RegisterAMF** | udm-uecm | PUT | AMF registration (3GPP/non-3GPP) |
| **DeregisterAMF** | udm-uecm | POST | AMF deregistration |
| **UpdatePEI** | udm-uecm | POST | PEI update |
| **UpdateRoamingInfo** | udm-uecm | POST | Roaming info update |
| **RegisterSMF** | udm-uecm | PUT | SMF registration per PDU session |
| **DeregisterSMF** | udm-uecm | DELETE | SMF deregistration |
| **RegisterSMSF** | udm-uecm | PUT | SMSF registration |
| **DeregisterSMSF** | udm-uecm | DELETE | SMSF deregistration |
| **RegisterIPSMGW** | udm-uecm | PUT | IP-SM-GW registration |
| **GetRegistrations** | udm-uecm | GET | Query all NF registrations |
| **SendRoutingInfoSM** | udm-uecm | POST | SMS routing info |
| **TriggerAuth** | udm-uecm | POST | Trigger re-authentication |
| **RestorePCSCF** | udm-uecm | POST | Restore P-CSCF for IMS |
| **RegisterNWDAF** | udm-uecm | PUT | NWDAF registration |
| **DeregisterNWDAF** | udm-uecm | DELETE | NWDAF deregistration |
| **GetLocation** | udm-uecm | GET | Get UE location registration |
| **CreateEESubscription** | udm-ee | POST | Create event subscription |
| **DeleteEESubscription** | udm-ee | DELETE | Delete event subscription |
| **ModifyEESubscription** | udm-ee | PATCH | Modify event subscription |
| **UpdatePPData** | udm-pp | PATCH | Update provisioned parameters |
| **Manage5GVNGroup** | udm-pp | PUT/PATCH/DELETE | 5G VN group CRUD |
| **ManagePPDataStore** | udm-pp | PUT/DELETE/GET | AF-specific PP data |
| **ManageMBSGroupMembership** | udm-pp | PUT/DELETE/GET | MBS group CRUD |
| **QueryUEInfo** | udm-mt | GET | UE location and state query |
| **ProvideLocationInfo** | udm-mt | POST | Provide UE location |
| **AuthorizeService** | udm-ssau | POST | Service-specific authorization |
| **RemoveAuthorization** | udm-ssau | POST | Remove authorization |
| **AuthorizeNIDD** | udm-niddau | POST | NIDD authorization |
| **ReportSMDeliveryStatus** | udm-rsds | POST | SMS delivery status report |

### 7.2 Data Ownership by Service

| Data Domain | Owner (Write) | Readers |
|------------|--------------|---------|
| Auth credentials (K, OPc, SQN) | udm-ueau | udm-ueau |
| Auth events | udm-ueau | udm-ee |
| Subscription profile (AM, SM, NSSAI) | udm-pp (provisioning) | udm-sdm, udm-ee |
| AMF registrations | udm-uecm | udm-sdm, udm-mt, udm-ee |
| SMF registrations | udm-uecm | udm-sdm |
| SMSF registrations | udm-uecm | udm-sdm, udm-mt |
| NWDAF registrations | udm-uecm | udm-uecm |
| EE subscriptions | udm-ee | udm-ee |
| SDM subscriptions | udm-sdm | udm-sdm, udm-notify |
| PP data | udm-pp | udm-sdm |
| 5G VN groups | udm-pp | udm-sdm |
| SUCI profiles (HPLMN keys) | O&M (external) | udm-ueid, udm-ueau |
| Service authorizations | udm-ssau | udm-ssau |
| SMS delivery status | udm-rsds | udm-ee |

---

## 8. Service Dependencies

### 8.1 Shared Component Dependencies

| Service | udm-common | udm-db | udm-cache | udm-notify | udm-nrf |
|---------|:----------:|:------:|:---------:|:----------:|:-------:|
| **udm-ueau** | ✅ | ✅ | ✅ | ❌ | ✅ |
| **udm-sdm** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **udm-uecm** | ✅ | ✅ | ✅ | ❌ | ✅ |
| **udm-ee** | ✅ | ✅ | ❌ | ✅ | ✅ |
| **udm-pp** | ✅ | ✅ | ❌ | ✅ | ✅ |
| **udm-mt** | ✅ | ✅ | ✅ | ❌ | ✅ |
| **udm-ssau** | ✅ | ✅ | ❌ | ❌ | ✅ |
| **udm-niddau** | ✅ | ✅ | ❌ | ❌ | ✅ |
| **udm-rsds** | ✅ | ✅ | ❌ | ✅ | ✅ |
| **udm-ueid** | ✅ | ✅ | ✅ | ❌ | ✅ |

**Legend**: ✅ = required dependency, ❌ = not used

**Rationale for Cache Usage**:
- **udm-ueau**: Caches SUCI profile (HPLMN keys) to avoid DB reads per auth request.
- **udm-sdm**: Caches subscriber profile data (AM, NSSAI) for high-frequency reads.
- **udm-uecm**: Caches serving AMF info for fast context lookups.
- **udm-mt**: Caches UE reachability state for MT routing.
- **udm-ueid**: Caches HPLMN key pairs for SUCI de-concealment.

### 8.2 Inter-Service Dependencies

| Service | Depends On | Dependency Type | Purpose |
|---------|-----------|----------------|---------|
| **udm-ueau** | udm-ueid | Sync HTTP | SUCI de-concealment |
| **udm-ee** | udm-uecm | Sync HTTP | Registration state for event correlation |
| **udm-ee** | udm-sdm | Sync HTTP | Subscriber data for event filtering |
| **udm-ee** | udm-notify | Library | Callback dispatch |
| **udm-mt** | udm-uecm | Sync HTTP | Serving AMF lookup |
| **udm-pp** | udm-sdm | Sync HTTP | Trigger SDM change notifications |
| **udm-pp** | udm-notify | Library | Change callback dispatch |
| **udm-sdm** | udm-notify | Library | Data change callback dispatch |
| **udm-rsds** | udm-notify | Library | Status event notification |

### 8.3 External Infrastructure Dependencies

| Infrastructure | Used By | Purpose |
|---------------|---------|---------|
| **YugabyteDB** | All services (via udm-db) | Persistent subscriber data storage |
| **Redis Cluster** | ueau, sdm, uecm, mt, ueid (via udm-cache) | Regional read cache |
| **NRF** | All services (via udm-nrf) | NF registration, discovery, OAuth2 tokens |
| **Kubernetes API** | All services | ConfigMap watch, health probes, pod lifecycle |
| **Istio / Service Mesh** | All services | mTLS, traffic management, observability |

### 8.4 Dependency Graph

```
                    ┌──────────────────────────────────┐
                    │        External NF Consumers      │
                    │    (AUSF, AMF, SMF, SMSF, NEF)   │
                    └────────────────┬─────────────────┘
                                     │
                    ┌────────────────▼─────────────────┐
                    │         UDM Services              │
                    │  ┌──────┬──────┬──────┬───────┐  │
                    │  │ ueau │ sdm  │ uecm │  ee   │  │
                    │  ├──────┼──────┼──────┼───────┤  │
                    │  │  pp  │  mt  │ ssau │niddau │  │
                    │  ├──────┼──────┤      │       │  │
                    │  │ rsds │ ueid │      │       │  │
                    │  └──┬───┴──┬───┴──┬───┴───┬───┘  │
                    └─────┼──────┼──────┼───────┼──────┘
                          │      │      │       │
              ┌───────────┼──────┼──────┼───────┼──────────────┐
              │           ▼      ▼      ▼       ▼              │
              │  ┌──────────┐ ┌──────┐ ┌──────┐ ┌──────────┐  │
              │  │udm-common│ │udm-db│ │cache │ │udm-notify│  │
              │  └──────────┘ └──┬───┘ └──┬───┘ └──────────┘  │
              │      Shared Libraries     │       │            │
              └───────────────────────────┼───────┼────────────┘
                                          │       │
                                    ┌─────▼──┐ ┌──▼────┐
                                    │Yugabyte│ │ Redis  │
                                    │   DB   │ │Cluster │
                                    └────────┘ └───────┘
```

---

## References

| Reference | Title |
|-----------|-------|
| [architecture.md](architecture.md) | UDM High-Level Architecture |
| 3GPP TS 29.503 | Nudm Services (all 11 service-based interfaces) |
| 3GPP TS 29.505 | Usage of the Unified Data Repository (subscription data schema) |
| 3GPP TS 29.500 | Technical Realization of Service Based Architecture |
| 3GPP TS 33.501 | Security Architecture and Procedures for 5G |
| 3GPP TS 23.501 | System Architecture for the 5G System |
| 3GPP TS 23.502 | Procedures for the 5G System |
| 3GPP TS 23.003 | Numbering, Addressing and Identification |

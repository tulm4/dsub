# SBI API Design — 5G UDM (Nudm) Services

> Comprehensive Service-Based Interface (SBI) API design documentation for the Unified Data
> Management (UDM) network function, covering all Nudm service APIs defined in 3GPP TS 29.503.

---

## Table of Contents

1. [SBI Overview](#1-sbi-overview)
2. [API Base URIs](#2-api-base-uris)
3. [Complete API Endpoint Catalog](#3-complete-api-endpoint-catalog)
4. [Sample API Requests and Responses](#4-sample-api-requests-and-responses)
5. [3GPP Custom HTTP Headers](#5-3gpp-custom-http-headers)
6. [Authentication and Authorization](#6-authentication-and-authorization)
7. [Error Handling](#7-error-handling)
8. [API Versioning Strategy](#8-api-versioning-strategy)
9. [Content Negotiation](#9-content-negotiation)
10. [Pagination](#10-pagination)
11. [Rate Limiting and Overload Control](#11-rate-limiting-and-overload-control)

---

## 1. SBI Overview

### What is SBI?

The **Service-Based Interface (SBI)** is the foundational communication paradigm of the 5G Core
(5GC) Service-Based Architecture (SBA). In contrast to the point-to-point reference-point model
used in earlier generations, every 5GC Network Function (NF) exposes its capabilities as a set of
**RESTful API services** that any authorized consumer NF can discover and invoke.

### Key Characteristics

| Property | Detail |
|---|---|
| **Transport** | HTTP/2 over TLS 1.2+ (mandatory in production) |
| **Serialization** | JSON (`application/json`) per IETF RFC 8259 |
| **API Style** | RESTful — resource-oriented with standard HTTP methods |
| **Discovery** | Dynamic via NRF (Network Repository Function) `Nnrf_NFDiscovery` |
| **Registration** | NFs register profiles with NRF via `Nnrf_NFManagement` |
| **Security** | OAuth 2.0 client-credentials + mutual TLS (mTLS) |
| **Specification** | 3GPP TS 29.500 (SBI framework), TS 29.501 (design principles) |

### UDM in the 5GC

The **Unified Data Management (UDM)** network function is the authoritative source for subscriber
data and authentication credentials. It exposes ten Nudm service APIs consumed by AMF, SMF, AUSF,
SMSF, NEF, NWDAF, and other NFs:

```
┌──────────┐   Nudm_UEAU    ┌─────────┐
│   AUSF   │───────────────▶│         │
└──────────┘                │         │   ┌──────┐
┌──────────┐   Nudm_SDM     │         │──▶│ UDR  │
│   AMF    │───────────────▶│   UDM   │   └──────┘
└──────────┘   Nudm_UECM   │         │
┌──────────┐───────────────▶│         │
│   SMF    │   Nudm_SDM     │         │
└──────────┘───────────────▶│         │
┌──────────┐   Nudm_EE      │         │
│   NEF    │───────────────▶│         │
└──────────┘                └─────────┘
```

---

## 2. API Base URIs

Every Nudm service is reachable at a base URI that follows the 3GPP convention:

```
{apiRoot}/nudm-{serviceName}/{apiVersion}/
```

where `{apiRoot}` is the scheme, authority, and optional deployment prefix
(e.g., `https://udm.5gc.mnc001.mcc001.3gppnetwork.org`).

| Service | Base URI | Version | Spec Reference |
|---|---|---|---|
| UE Authentication | `{apiRoot}/nudm-ueau/v1/` | v1.3.2 | TS29503\_Nudm\_UEAU |
| Subscriber Data Management | `{apiRoot}/nudm-sdm/v2/` | v2.3.6 | TS29503\_Nudm\_SDM |
| UE Context Management | `{apiRoot}/nudm-uecm/v1/` | v1.3.3 | TS29503\_Nudm\_UECM |
| Event Exposure | `{apiRoot}/nudm-ee/v1/` | v1.3.1 | TS29503\_Nudm\_EE |
| Parameter Provisioning | `{apiRoot}/nudm-pp/v1/` | v1.3.3 | TS29503\_Nudm\_PP |
| Mobile Terminated | `{apiRoot}/nudm-mt/v1/` | v1.2.0 | TS29503\_Nudm\_MT |
| Service-Specific Auth | `{apiRoot}/nudm-ssau/v1/` | v1.1.1 | TS29503\_Nudm\_SSAU |
| NIDD Authorization | `{apiRoot}/nudm-niddau/v1/` | v1.2.0 | TS29503\_Nudm\_NIDDAU |
| Report SMS Delivery Status | `{apiRoot}/nudm-rsds/v1/` | v1.2.0 | TS29503\_Nudm\_RSDS |
| UE Identifier | `{apiRoot}/nudm-ueid/v1/` | v1.0.0 | TS29503\_Nudm\_UEID |

---

## 3. Complete API Endpoint Catalog

### 3.1 Nudm\_UEAU — UE Authentication (7 endpoints)

Primary consumers: **AUSF**, **HSS interworking**

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/{supiOrSuci}/security-information/generate-auth-data` | GenerateAuthData | Generate 5G-AKA or EAP-AKA' authentication vectors |
| GET | `/{supiOrSuci}/security-information-rg` | GetRgAuthData | Retrieve authentication data for Residential Gateway |
| POST | `/{supi}/auth-events` | ConfirmAuth | Create an authentication confirmation event |
| PUT | `/{supi}/auth-events/{authEventId}` | DeleteAuth | Delete (overwrite) an authentication result (PUT per 3GPP spec — replaces the resource) |
| POST | `/{supi}/hss-security-information/{hssAuthType}/generate-av` | GenerateAv | Generate authentication vectors for HSS interworking |
| POST | `/{supi}/gba-security-information/generate-av` | GenerateGbaAv | Generate GBA authentication vectors |
| POST | `/{supiOrSuci}/prose-security-information/generate-av` | GenerateProseAV | Generate ProSe authentication vectors |

**Key Schemas:** `AuthenticationInfoRequest`, `AuthenticationInfoResult`, `AuthenticationVector`,
`RgAuthCtx`, `AuthEvent`, `ResynchronizationInfo`, `HssAuthenticationVectors`

---

### 3.2 Nudm\_SDM — Subscriber Data Management (40 endpoints)

Primary consumers: **AMF**, **SMF**, **SMSF**, **NWDAF**, **NEF**, **AF** (via NEF)

#### Multi-Dataset Retrieval

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}` | GetDataSets | Retrieve multiple subscription data sets in one request |

#### Access and Mobility Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/am-data` | GetAmData | Get access and mobility subscription data |
| GET | `/{supi}/am-data/ecr-data` | GetEcrData | Get enhanced coverage restriction data |
| PUT | `/{supi}/am-data/sor-ack` | SorAckInfo | Acknowledge Steering of Roaming information |
| PUT | `/{supi}/am-data/upu-ack` | UpuAck | Acknowledge UE Parameters Update |
| PUT | `/{supi}/am-data/subscribed-snssais-ack` | SubscribedSnssaisAck | Acknowledge subscribed S-NSSAIs |
| PUT | `/{supi}/am-data/cag-ack` | CagAck | Acknowledge Closed Access Group information |
| POST | `/{supi}/am-data/update-sor` | UpdateSorInfo | Update Steering of Roaming information |

#### Session Management Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/smf-select-data` | GetSmfSelData | Get SMF selection subscription data |
| GET | `/{supi}/sm-data` | GetSmData | Get session management subscription data |

#### Messaging Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/sms-data` | GetSmsData | Get SMS subscription data |
| GET | `/{supi}/sms-mng-data` | GetSmsMngtData | Get SMS management subscription data |

#### Network Slice and Identifiers

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/nssai` | GetNSSAI | Get subscribed network slice selection data |
| GET | `/{ueId}/id-translation-result` | GetSupiOrGpsi | Translate between SUPI and GPSI identifiers |

#### Location and Context

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/ue-context-in-amf-data` | GetUeCtxInAmfData | Get UE context info held by AMF |
| GET | `/{supi}/ue-context-in-smf-data` | GetUeCtxInSmfData | Get UE context info held by SMF |
| GET | `/{supi}/ue-context-in-smsf-data` | GetUeCtxInSmsfData | Get UE context info held by SMSF |

#### LCS (Location Services) Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/lcs-subscription-data` | GetLcsSubscriptionData | Get LCS subscription data |
| GET | `/{supi}/lcs-mo-data` | GetLcsMoData | Get LCS mobile-originated data |
| GET | `/{supi}/lcs-bca-data` | GetLcsBcaData | Get LCS broadcast assistance data |
| GET | `/{ueId}/lcs-privacy-data` | GetLcsPrivacyData | Get LCS privacy subscription data |

#### V2X, ProSe, and Sidelink Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/v2x-data` | GetV2xData | Get V2X subscription data |
| GET | `/{supi}/prose-data` | GetProseData | Get ProSe (Proximity Services) data |
| GET | `/{supi}/ranging-slpos-data` | GetRangingSlPosData | Get ranging/sidelink positioning data |
| GET | `/{ueId}/rangingsl-privacy-data` | GetRangingSlPrivacyData | Get ranging/SL privacy data |
| GET | `/{supi}/a2x-data` | GetA2xData | Get A2X (Aircraft-to-Everything) data |

#### MBS, Time Sync, Trace, and UC Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}/5mbs-data` | GetMbsData | Get MBS (Multicast/Broadcast) subscription data |
| GET | `/{supi}/time-sync-data` | GetTimeSyncSubscriptionData | Get time synchronization subscription data |
| GET | `/{supi}/trace-data` | GetTraceConfigData | Get trace configuration data |
| GET | `/{supi}/uc-data` | GetUcData | Get user consent data |

#### Subscription Management

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/{ueId}/sdm-subscriptions` | Subscribe | Create a subscription to data change notifications |
| PATCH | `/{ueId}/sdm-subscriptions/{subscriptionId}` | Modify | Modify an existing SDM subscription |
| DELETE | `/{ueId}/sdm-subscriptions/{subscriptionId}` | Unsubscribe | Remove an SDM subscription |

#### Shared Data

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/shared-data` | GetSharedData | Retrieve shared subscription data by IDs |
| GET | `/shared-data/{sharedDataId}` | GetIndividualSharedData | Retrieve a single shared data entry |
| POST | `/shared-data-subscriptions` | SubscribeToSharedData | Subscribe to shared data changes |
| PATCH | `/shared-data-subscriptions/{subscriptionId}` | ModifySharedDataSubs | Modify shared data subscription |
| DELETE | `/shared-data-subscriptions/{subscriptionId}` | UnsubscribeForSharedData | Remove shared data subscription |

#### Group and Multi-Identifier Operations

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/group-data/group-identifiers` | GetGroupIdentifiers | Get group identifiers for an external group ID |
| GET | `/multiple-identifiers` | GetMultipleIdentifiers | Get multiple SUPI/GPSI mappings |

**Key Schemas:** `AccessAndMobilitySubscriptionData`, `SessionManagementSubscriptionData`,
`SmfSelectionSubscriptionData`, `SmsSubscriptionData`, `Nssai`, `SubscriptionDataSets`,
`SdmSubscription`, `SharedData`, `IdTranslationResult`, `LcsPrivacyData`

---

### 3.3 Nudm\_UECM — UE Context Management (34 endpoints)

Primary consumers: **AMF**, **SMF**, **SMSF**, **IP-SM-GW**, **NWDAF**

#### Registration Overview

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{ueId}/registrations` | GetRegistrations | Retrieve all registration data sets for a UE |
| POST | `/{ueId}/registrations/send-routing-info-sm` | SendRoutingInfoSm | Get SM routing info (for SMS-over-NAS) |

#### 3GPP Access AMF Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/{ueId}/registrations/amf-3gpp-access` | 3GppRegistration | Register AMF for 3GPP access |
| GET | `/{ueId}/registrations/amf-3gpp-access` | Get3GppRegistration | Get current AMF 3GPP registration |
| PATCH | `/{ueId}/registrations/amf-3gpp-access` | Update3GppRegistration | Update AMF 3GPP registration |
| POST | `/{ueId}/registrations/amf-3gpp-access/dereg-amf` | DeregAMF | Trigger AMF deregistration |
| POST | `/{ueId}/registrations/amf-3gpp-access/pei-update` | PeiUpdate | Update PEI (Permanent Equipment Identifier) |
| POST | `/{ueId}/registrations/amf-3gpp-access/roaming-info-update` | UpdateRoamingInformation | Update roaming information |

#### Non-3GPP Access AMF Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/{ueId}/registrations/amf-non-3gpp-access` | Non3GppRegistration | Register AMF for non-3GPP access |
| GET | `/{ueId}/registrations/amf-non-3gpp-access` | GetNon3GppRegistration | Get AMF non-3GPP registration |
| PATCH | `/{ueId}/registrations/amf-non-3gpp-access` | UpdateNon3GppRegistration | Update AMF non-3GPP registration |

#### SMF Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{ueId}/registrations/smf-registrations` | GetSmfRegistration | List all SMF registrations |
| GET | `/{ueId}/registrations/smf-registrations/{pduSessionId}` | RetrieveSmfRegistration | Get a specific SMF registration |
| PUT | `/{ueId}/registrations/smf-registrations/{pduSessionId}` | Registration | Register SMF for a PDU session |
| PATCH | `/{ueId}/registrations/smf-registrations/{pduSessionId}` | UpdateSmfRegistration | Update SMF registration |
| DELETE | `/{ueId}/registrations/smf-registrations/{pduSessionId}` | SmfDeregistration | Deregister SMF for a PDU session |

#### SMSF 3GPP Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/{ueId}/registrations/smsf-3gpp-access` | 3GppSmsfRegistration | Register SMSF for 3GPP access |
| GET | `/{ueId}/registrations/smsf-3gpp-access` | Get3GppSmsfRegistration | Get SMSF 3GPP registration |
| PATCH | `/{ueId}/registrations/smsf-3gpp-access` | UpdateSmsf3GppRegistration | Update SMSF 3GPP registration |
| DELETE | `/{ueId}/registrations/smsf-3gpp-access` | 3GppSmsfDeregistration | Deregister SMSF for 3GPP access |

#### SMSF Non-3GPP Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/{ueId}/registrations/smsf-non-3gpp-access` | Non3GppSmsfRegistration | Register SMSF for non-3GPP access |
| GET | `/{ueId}/registrations/smsf-non-3gpp-access` | GetNon3GppSmsfRegistration | Get SMSF non-3GPP registration |
| PATCH | `/{ueId}/registrations/smsf-non-3gpp-access` | UpdateSmsfNon3GppRegistration | Update SMSF non-3GPP registration |
| DELETE | `/{ueId}/registrations/smsf-non-3gpp-access` | Non3GppSmsfDeregistration | Deregister SMSF for non-3GPP access |

#### IP-SM-GW Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/{ueId}/registrations/ip-sm-gw` | IpSmGwRegistration | Register IP-SM-GW |
| GET | `/{ueId}/registrations/ip-sm-gw` | GetIpSmGwRegistration | Get IP-SM-GW registration |
| DELETE | `/{ueId}/registrations/ip-sm-gw` | IpSmGwDeregistration | Deregister IP-SM-GW |

#### NWDAF Registration

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{ueId}/registrations/nwdaf-registrations` | GetNwdafRegistration | List NWDAF registrations |
| PUT | `/{ueId}/registrations/nwdaf-registrations/{nwdafRegistrationId}` | NwdafRegistration | Register NWDAF |
| PATCH | `/{ueId}/registrations/nwdaf-registrations/{nwdafRegistrationId}` | UpdateNwdafRegistration | Update NWDAF registration |
| DELETE | `/{ueId}/registrations/nwdaf-registrations/{nwdafRegistrationId}` | NwdafDeregistration | Deregister NWDAF |

#### Location and Other

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{ueId}/registrations/location` | GetLocationInfo | Get UE location information |
| GET | `/{ueId}/registrations/trigger-auth` | AuthTrigger | Trigger authentication authorization (GET per 3GPP spec — retrieves authorization status) |
| POST | `/restore-pcscf` | RestorePcscf | Trigger P-CSCF restoration |

**Key Schemas:** `Amf3GppAccessRegistration`, `AmfNon3GppAccessRegistration`,
`SmfRegistration`, `SmsfRegistration`, `IpSmGwRegistration`, `NwdafRegistration`,
`RegistrationDataSets`, `DeregistrationData`

---

### 3.4 Nudm\_EE — Event Exposure (3 endpoints)

Primary consumers: **NEF**, **NWDAF**, **GMLC**

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/{ueIdentity}/ee-subscriptions` | CreateEeSubscription | Subscribe to UE monitoring events |
| PATCH | `/{ueIdentity}/ee-subscriptions/{subscriptionId}` | UpdateEeSubscription | Modify an existing event subscription |
| DELETE | `/{ueIdentity}/ee-subscriptions/{subscriptionId}` | DeleteEeSubscription | Remove an event subscription |

**Supported Event Types:** Loss of connectivity, UE reachability, location reporting, change of
SUPI-PEI association, roaming status, communication failure, availability after DDN failure,
CN type change, DL data delivery status, PDN connectivity status, idle status indication.

**Key Schemas:** `EeSubscription`, `MonitoringConfiguration`, `MonitoringReport`, `EventType`,
`CreatedEeSubscription`, `EeSubscriptionError`

---

### 3.5 Nudm\_PP — Parameter Provisioning (12 endpoints)

Primary consumers: **NEF**, **AF** (via NEF)

#### UE-Level Parameter Provisioning

| Method | Path | Operation | Description |
|---|---|---|---|
| PATCH | `/{ueId}/pp-data` | Update | Update provisioned parameter data |
| PUT | `/{ueId}/pp-data-store/{afInstanceId}` | CreatePPEntry | Create a PP data store entry for an AF |
| GET | `/{ueId}/pp-data-store/{afInstanceId}` | GetPPEntry | Retrieve a PP data store entry |
| DELETE | `/{ueId}/pp-data-store/{afInstanceId}` | DeletePPEntry | Delete a PP data store entry |

#### 5G VN Group Configuration

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/5g-vn-groups/{extGroupId}` | Create5GVnGroup | Create 5G VN group configuration |
| GET | `/5g-vn-groups/{extGroupId}` | Get5GVnGroup | Get 5G VN group configuration |
| PATCH | `/5g-vn-groups/{extGroupId}` | Modify5GVnGroup | Modify 5G VN group configuration |
| DELETE | `/5g-vn-groups/{extGroupId}` | Delete5GVnGroup | Delete 5G VN group configuration |

#### MBS Group Membership

| Method | Path | Operation | Description |
|---|---|---|---|
| PUT | `/mbs-group-membership/{extGroupId}` | CreateMbsGroupMembership | Create MBS group membership |
| GET | `/mbs-group-membership/{extGroupId}` | GetMbsGroupMembership | Get MBS group membership |
| PATCH | `/mbs-group-membership/{extGroupId}` | ModifyMbsGroupMembership | Modify MBS group membership |
| DELETE | `/mbs-group-membership/{extGroupId}` | DeleteMbsGroupMembership | Delete MBS group membership |

**Key Schemas:** `PpData`, `PpDataEntry`, `5GVnGroupConfiguration`, `MbsGroupMemb`

---

### 3.6 Nudm\_MT — Mobile Terminated (2 endpoints)

Primary consumers: **SMF**, **SMSF**

| Method | Path | Operation | Description |
|---|---|---|---|
| GET | `/{supi}` | QueryUeInfo | Query UE reachability and location info |
| POST | `/{supi}/loc-info/provide-loc-info` | ProvideLocationInfo | Provide UE location information |

**Key Schemas:** `UeInfo`, `LocationInfoRequest`, `LocationInfoResult`

---

### 3.7 Nudm\_SSAU — Service-Specific Authorization (2 endpoints)

Primary consumers: **NEF**, **AF** (via NEF)

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/{ueIdentity}/{serviceType}/authorize` | ServiceSpecificAuthorization | Authorize service-specific parameters |
| POST | `/{ueIdentity}/{serviceType}/remove` | ServiceSpecificAuthorizationRemoval | Remove service-specific authorization |

**Service Types:** `AF_GUIDANCE_FOR_URSP`, `AF_REQUESTED_QOS`

**Key Schemas:** `ServiceSpecificAuthorizationInfo`, `ServiceSpecificAuthorizationData`,
`AuthUpdateNotification`

---

### 3.8 Nudm\_NIDDAU — NIDD Authorization (1 endpoint)

Primary consumers: **NEF**

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/{ueIdentity}/authorize` | AuthorizeNiddData | Authorize NIDD (Non-IP Data Delivery) configuration |

**Key Schemas:** `AuthorizationInfo`, `AuthorizationData`, `NiddAuthUpdateNotification`

---

### 3.9 Nudm\_RSDS — Report SMS Delivery Status (1 endpoint)

Primary consumers: **SMSF**

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/{ueIdentity}/sm-delivery-status` | ReportSMDeliveryStatus | Report SMS delivery status to UDM |

**Key Schemas:** `SmDeliveryStatus`

---

### 3.10 Nudm\_UEID — UE Identifier (1 endpoint)

Primary consumers: **AUSF**, **AMF**

| Method | Path | Operation | Description |
|---|---|---|---|
| POST | `/deconceal` | Deconceal | Resolve a SUCI to the permanent SUPI |

**Key Schemas:** `DeconcealReqData`, `DeconcealRspData`

---

## 4. Sample API Requests and Responses

### 4.1 Generate Authentication Data (UEAU)

**Request:**

```http
POST /nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data HTTP/2
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...

{
  "servingNetworkName": "5G:mnc001.mcc001.3gppnetwork.org",
  "ausfInstanceId": "a1b2c3d4-5678-9abc-def0-123456789abc",
  "resynchronizationInfo": {
    "rand": "4FCA7C1E3B5D8A90F2C6D7E1A0B3C4D5",
    "auts": "A1B2C3D4E5F6A1B2C3D4E5F6A7B8"
  }
}
```

**Response (200 OK):**

```json
{
  "authType": "5G_AKA",
  "authenticationVector": {
    "avType": "5G_HE_AKA",
    "rand": "4FCA7C1E3B5D8A90F2C6D7E1A0B3C4D5",
    "autn": "D4E5F6A7B8C9D0E1F2A3B4C5D6E7F8A9",
    "xresStar": "A0B1C2D3E4F5A6B7C8D9E0F1A2B3C4D5",
    "kausf": "0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"
  },
  "supi": "imsi-001010000000001"
}
```

---

### 4.2 Get Access and Mobility Subscription Data (SDM)

**Request:**

```http
GET /nudm-sdm/v2/imsi-001010000000001/am-data?plmn-id=%7B%22mcc%22%3A%22001%22%2C%22mnc%22%3A%2201%22%7D HTTP/2
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
Accept: application/json
```

**Response (200 OK):**

```json
{
  "gpsis": [
    "msisdn-14155551234"
  ],
  "subscribedUeAmbr": {
    "uplink": "100 Mbps",
    "downlink": "250 Mbps"
  },
  "nssai": {
    "defaultSingleNssais": [
      {
        "sst": 1,
        "sd": "000001"
      }
    ],
    "singleNssais": [
      {
        "sst": 1,
        "sd": "000001"
      },
      {
        "sst": 2,
        "sd": "000002"
      }
    ]
  },
  "ratRestrictions": [
    "NR"
  ],
  "forbiddenAreas": [],
  "serviceAreaRestriction": {
    "restrictionType": "ALLOWED_AREAS",
    "areas": [
      {
        "tacs": ["000001", "000002"]
      }
    ]
  }
}
```

---

### 4.3 Register AMF Context (UECM)

**Request:**

```http
PUT /nudm-uecm/v1/imsi-001010000000001/registrations/amf-3gpp-access HTTP/2
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...

{
  "amfInstanceId": "b2c3d4e5-6789-abcd-ef01-23456789abcd",
  "deregCallbackUri": "https://amf.5gc.mnc001.mcc001.3gppnetwork.org/namf-callback/v1/imsi-001010000000001/dereg-notify",
  "guami": {
    "plmnId": {
      "mcc": "001",
      "mnc": "01"
    },
    "amfId": "020040"
  },
  "ratType": "NR",
  "initialRegistrationInd": true,
  "supportedFeatures": "0"
}
```

**Response (200 OK or 201 Created):**

```json
{
  "amfInstanceId": "b2c3d4e5-6789-abcd-ef01-23456789abcd",
  "deregCallbackUri": "https://amf.5gc.mnc001.mcc001.3gppnetwork.org/namf-callback/v1/imsi-001010000000001/dereg-notify",
  "guami": {
    "plmnId": {
      "mcc": "001",
      "mnc": "01"
    },
    "amfId": "020040"
  },
  "ratType": "NR"
}
```

---

### 4.4 Deconceal SUCI (UEID)

**Request:**

```http
POST /nudm-ueid/v1/deconceal HTTP/2
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...

{
  "suci": "suci-0-001-01-home-network-0-0-0000000001"
}
```

**Response (200 OK):**

```json
{
  "supi": "imsi-001010000000001"
}
```

---

### 4.5 Create EE Subscription

**Request:**

```http
POST /nudm-ee/v1/imsi-001010000000001/ee-subscriptions HTTP/2
Content-Type: application/json
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...

{
  "callbackReference": "https://nef.5gc.mnc001.mcc001.3gppnetwork.org/nnef-callback/v1/ee-notify",
  "monitoringConfigurations": {
    "config1": {
      "eventType": "LOSS_OF_CONNECTIVITY",
      "immediateFlag": true,
      "maximumLatency": 30,
      "maximumResponseTime": 60
    }
  },
  "reportingOptions": {
    "maxNumOfReports": 10
  },
  "supportedFeatures": "0"
}
```

**Response (201 Created):**

```json
{
  "callbackReference": "https://nef.5gc.mnc001.mcc001.3gppnetwork.org/nnef-callback/v1/ee-notify",
  "monitoringConfigurations": {
    "config1": {
      "eventType": "LOSS_OF_CONNECTIVITY",
      "immediateFlag": true
    }
  },
  "monitoringReport": [
    {
      "referenceId": "config1",
      "eventType": "LOSS_OF_CONNECTIVITY",
      "report": {
        "lossOfConnectReason": 1
      },
      "reachability": "UNREACHABLE"
    }
  ],
  "subscriptionId": "sub-ee-00001"
}
```

---

### 4.6 Get SMF Selection Data (SDM)

**Request:**

```http
GET /nudm-sdm/v2/imsi-001010000000001/smf-select-data?plmn-id=%7B%22mcc%22%3A%22001%22%2C%22mnc%22%3A%2201%22%7D HTTP/2
Authorization: Bearer eyJhbGciOiJSUzI1NiIs...
```

**Response (200 OK):**

```json
{
  "subscribedSnssaiInfos": {
    "{\"sst\":1,\"sd\":\"000001\"}": {
      "dnnInfos": [
        {
          "dnn": "internet",
          "defaultDnnIndicator": true
        },
        {
          "dnn": "ims"
        }
      ]
    }
  },
  "sharedSnssaiInfosId": "shared-snssai-001"
}
```

---

### 4.7 Subscribe to Data Change Notifications (SDM)

**Request:**

```http
POST /nudm-sdm/v2/imsi-001010000000001/sdm-subscriptions HTTP/2
Content-Type: application/json

{
  "nfInstanceId": "c3d4e5f6-789a-bcde-f012-3456789abcde",
  "callbackReference": "https://amf.example.com/nudm-sdm-notify",
  "monitoredResourceUris": [
    "/nudm-sdm/v2/imsi-001010000000001/am-data",
    "/nudm-sdm/v2/imsi-001010000000001/smf-select-data"
  ],
  "supportedFeatures": "0"
}
```

**Response (201 Created):**

```json
{
  "nfInstanceId": "c3d4e5f6-789a-bcde-f012-3456789abcde",
  "callbackReference": "https://amf.example.com/nudm-sdm-notify",
  "monitoredResourceUris": [
    "/nudm-sdm/v2/imsi-001010000000001/am-data",
    "/nudm-sdm/v2/imsi-001010000000001/smf-select-data"
  ],
  "subscriptionId": "sdm-sub-001"
}
```

---

## 5. 3GPP Custom HTTP Headers

The 3GPP SBI framework defines a rich set of custom headers (per TS 29.500, codified in
`TS29500_CustomHeaders.abnf` version 18.6.1). These headers carry signaling metadata that
HTTP/2 alone cannot express.

### 5.1 Message Priority and Timing

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Message-Priority` | Indicates the relative priority of the request (0–31, lower is higher priority). Used by SCP/SEPP for request scheduling. | `3gpp-Sbi-Message-Priority: 5` |
| `3gpp-Sbi-Sender-Timestamp` | Timestamp when the request was sent, including milliseconds (RFC 5322 date format with `GMT`). Enables latency measurement. | `3gpp-Sbi-Sender-Timestamp: Tue, 04 Jun 2024 10:30:00.123 GMT` |
| `3gpp-Sbi-Max-Rsp-Time` | Maximum response time in milliseconds the consumer is willing to wait. | `3gpp-Sbi-Max-Rsp-Time: 2000` |

### 5.2 Routing and Binding

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Target-apiRoot` | The API root of the target NF. Used by SCP for indirect communication to route the request to the correct producer. | `3gpp-Sbi-Target-apiRoot: https://udm1.5gc.mnc001.mcc001.3gppnetwork.org` |
| `3gpp-Sbi-Routing-Binding` | Carries binding indication with level (`NF_INSTANCE`, `NF_SET`, `NF_SERVICE_INSTANCE`, `NF_SERVICE_SET`) and associated parameters. Ensures session stickiness. | `3gpp-Sbi-Routing-Binding: bl=NF_INSTANCE; nfinst=<uuid>` |
| `3gpp-Sbi-Binding` | Binding element with recovery time, notification receiver info, and optional no-redundancy flag. Created by producer to maintain context affinity. | `3gpp-Sbi-Binding: bl=NF_INSTANCE; nfinst=<uuid>; recoverytime=2024-06-04T10:30:00Z` |

### 5.3 Producer and Consumer Identity

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Producer-Id` | Identifies the producer NF. Contains NF instance ID, service instance ID, NF set ID, and/or service set ID. | `3gpp-Sbi-Producer-Id: nfinst=<uuid>; nfservinst=<uuid>` |
| `3gpp-Sbi-Target-Nf-Id` | Identifies the target NF instance and optional service instance. Used by SCP for request routing. | `3gpp-Sbi-Target-Nf-Id: nfinst=<uuid>` |
| `3gpp-Sbi-Target-Nf-Group-Id` | Target NF group identifier for group-level routing. | `3gpp-Sbi-Target-Nf-Group-Id: udm-group-001` |
| `3gpp-Sbi-NF-Peer-Info` | Contains source and destination NF identifiers, service instances, SCP/SEPP FQDNs. Facilitates end-to-end tracing across intermediaries. | `3gpp-Sbi-NF-Peer-Info: srcinst=<uuid>; dstinst=<uuid>` |
| `3gpp-Sbi-Consumer-Info` | Consumer service details including API version, supported features, accepted encodings, and callback API roots. | `3gpp-Sbi-Consumer-Info: svc=nausf-auth; apiver=v1` |

### 5.4 Overload and Load Control

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Oci` | **Overload Control Indication.** Sent by an overloaded producer or intermediary. Contains timestamp, validity period (seconds), overload reduction metric (0–100 percentage), and scope. Consumers must reduce traffic accordingly. | `3gpp-Sbi-Oci: ts=2024-06-04T10:30:00Z; validity=60; metric=50` |
| `3gpp-Sbi-Lci` | **Load Control Indication.** Sent by producer to advertise current load level. Contains timestamp, load metric (0–100), and scope. Consumers use this for load-aware routing. | `3gpp-Sbi-Lci: ts=2024-06-04T10:30:00Z; metric=75` |

### 5.5 Security and Authorization

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Client-Credentials` | JWT-formatted client credentials for service authorization between NFs. | `3gpp-Sbi-Client-Credentials: eyJhbGciOi...` |
| `3gpp-Sbi-Source-NF-Client-Credentials` | Source NF client credentials (JWT) when request traverses an intermediary (SCP). | `3gpp-Sbi-Source-NF-Client-Credentials: eyJhbGciOi...` |
| `3gpp-Sbi-Access-Scope` | OAuth2 access scopes granted to the consumer, space-separated. | `3gpp-Sbi-Access-Scope: nudm-sdm nudm-uecm` |
| `3gpp-Sbi-Other-Access-Scopes` | Additional access scopes beyond the primary scope. | `3gpp-Sbi-Other-Access-Scopes: nudm-ee` |
| `3gpp-Sbi-Access-Token` | Bearer access token in credentials format for service authorization. | `3gpp-Sbi-Access-Token: eyJhbGciOi...` |

### 5.6 NRF and Discovery

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Nrf-Uri` | NRF endpoint URIs for discovery (`nnrf-disc`), management (`nnrf-nfm`), and OAuth2 (`nnrf-oauth2`). Optionally lists `oauth2-requested-services`. | `3gpp-Sbi-Nrf-Uri: nnrf-disc=https://nrf.example.com/nnrf-disc/v1` |
| `3gpp-Sbi-Nrf-Uri-Callback` | NRF URIs for callback-related discovery and management. | `3gpp-Sbi-Nrf-Uri-Callback: nnrf-disc=https://nrf.example.com/nnrf-disc/v1` |

### 5.7 Callback and Notification

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Callback` | Indicates the type of callback being invoked. Carries the callback name and optional API version. | `3gpp-Sbi-Callback: Nudm_SDM_Notification; apiver=v2` |
| `3gpp-Sbi-Notif-Accepted-Encoding` | Content encodings the notification receiver accepts (e.g., `gzip`, `deflate`), with optional quality weights. | `3gpp-Sbi-Notif-Accepted-Encoding: gzip;q=1.0, identity;q=0.5` |

### 5.8 Correlation and Tracing

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Correlation-Info` | Subscriber correlation identifiers for distributed tracing. Supports types: `imsi`, `impi`, `suci`, `nai`, `gci`, `gli`, `impu`, `msisdn`, `extid`, `imeisv`, `imei`, `mac`, `eui`. | `3gpp-Sbi-Correlation-Info: imsi-001010000000001` |

### 5.9 Inter-PLMN and Roaming

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Originating-Network-Id` | Originating PLMN identity (MCC-MNC) with optional source type (SCP/SEPP) and FQDN. Used in roaming scenarios. | `3gpp-Sbi-Originating-Network-Id: 001-01; srctype=SEPP` |
| `3gpp-Sbi-Interplmn-Purpose` | Purpose of inter-PLMN communication. Values: `ROAMING`, `INTER_PLMN_MOBILITY`, `SMS_INTERCONNECT`, `SNPN_INTERCONNECT`, `DISASTER_ROAMING`, `DATA_ANALYTICS_EXCHANGE`. | `3gpp-Sbi-Interplmn-Purpose: ROAMING` |

### 5.10 Hop Control, Selection, and Request/Response Metadata

| Header | Purpose | Example |
|---|---|---|
| `3gpp-Sbi-Max-Forward-Hops` | Maximum number of SCP/SEPP hops allowed for the request, with optional node type. Prevents routing loops. | `3gpp-Sbi-Max-Forward-Hops: 3` |
| `3gpp-Sbi-Selection-Info` | Routing selection criteria: reselection flag, exclusion of specific NF instances/sets/services. | `3gpp-Sbi-Selection-Info: reselection=true` |
| `3gpp-Sbi-Request-Info` | Request metadata: retransmission flag, redirect indicator, reason, idempotency key, callback URI prefix. | `3gpp-Sbi-Request-Info: retrans=true; idempotency-key=abc-123` |
| `3gpp-Sbi-Response-Info` | Response metadata: whether the request was retransmitted, context transferred, no-retry indicator, NF/service identifiers. | `3gpp-Sbi-Response-Info: nfinst=<uuid>` |
| `3gpp-Sbi-Retry-Info` | Retry control indication (e.g., `no-retries` to signal that the consumer should not retry). | `3gpp-Sbi-Retry-Info: no-retries` |
| `3gpp-Sbi-Alternate-Chf-Id` | Alternate CHF (Charging Function) instance for failover, with primary/secondary designation. | `3gpp-Sbi-Alternate-Chf-Id: nfinst=<uuid>; type=primary` |

---

## 6. Authentication and Authorization

### 6.1 OAuth 2.0 Client Credentials Flow

All Nudm service consumers obtain access tokens from the **NRF** (acting as the OAuth 2.0
Authorization Server) using the **Client Credentials** grant type (RFC 6749 §4.4).

```
┌──────────┐                       ┌──────────┐                    ┌──────────┐
│ Consumer  │──(1) Token Request──▶│   NRF    │                    │   UDM    │
│   NF     │◀─(2) Access Token────│ (AuthZ)  │                    │(Producer)│
│ (e.g.AMF)│──(3) API Request + Bearer Token─────────────────────▶│          │
│          │◀─(4) API Response────────────────────────────────────│          │
└──────────┘                       └──────────┘                    └──────────┘
```

**Step 1 — Token Request:**

```http
POST /oauth2/token HTTP/2
Host: nrf.5gc.mnc001.mcc001.3gppnetwork.org
Content-Type: application/x-www-form-urlencoded

grant_type=client_credentials
&nfInstanceId=b2c3d4e5-6789-abcd-ef01-23456789abcd
&nfType=AMF
&targetNfType=UDM
&scope=nudm-sdm
&targetNfInstanceId=a1b2c3d4-5678-9abc-def0-123456789abc
```

**Step 2 — Token Response:**

```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "scope": "nudm-sdm"
}
```

### 6.2 Per-Service OAuth2 Scopes

Each Nudm service defines its own OAuth2 scope. Some services define fine-grained sub-scopes
for individual operations.

| Service | Scope | Example Sub-Scopes |
|---|---|---|
| UE Authentication | `nudm-ueau` | — |
| Subscriber Data Management | `nudm-sdm` | `nudm-sdm:multi-data-sets:read`, `nudm-sdm:subscription:create` |
| UE Context Management | `nudm-uecm` | — |
| Event Exposure | `nudm-ee` | — |
| Parameter Provisioning | `nudm-pp` | — |
| Mobile Terminated | `nudm-mt` | — |
| Service-Specific Auth | `nudm-ssau` | — |
| NIDD Authorization | `nudm-niddau` | — |
| Report SMS Delivery Status | `nudm-rsds` | — |
| UE Identifier | `nudm-ueid` | — |

### 6.3 Token Validation

The UDM producer validates incoming access tokens by:

1. Verifying the JWT signature against the NRF's public key
2. Checking `exp` (expiration), `iat` (issued-at), and `nbf` (not-before) claims
3. Confirming the `scope` claim includes the required service scope
4. Validating `iss` (issuer) matches the expected NRF instance
5. Optionally checking `aud` (audience) matches the UDM NF instance or type

### 6.4 Mutual TLS (mTLS)

All SBI communication is protected by TLS 1.2 or higher with mutual authentication:

- **Server certificate:** The UDM presents a certificate signed by a trusted 5GC PKI CA
- **Client certificate:** Consumer NFs present their own certificate for mutual authentication
- **Certificate validation:** Both sides verify the peer certificate chain against the trusted CA
- **PLMN trust:** In roaming scenarios, SEPP-to-SEPP communication uses separate TLS domains
  with N32 interface protection (JWE/JWS or TLS)

---

## 7. Error Handling

### 7.1 ProblemDetails (RFC 7807)

All Nudm APIs return errors using the `application/problem+json` media type as defined in
RFC 7807 and extended by 3GPP TS 29.500.

**Error Response Structure:**

```json
{
  "type": "https://example.com/problems/authentication-failure",
  "title": "Authentication Data Generation Failed",
  "status": 403,
  "detail": "The subscriber's authentication subscription is not found in the UDR.",
  "instance": "/nudm-ueau/v1/imsi-001010000000001/security-information/generate-auth-data",
  "cause": "AUTHENTICATION_REJECTED",
  "invalidParams": [
    {
      "param": "servingNetworkName",
      "reason": "Serving network not authorized for this subscriber"
    }
  ],
  "supportedFeatures": "0"
}
```

### 7.2 Standard HTTP Status Codes

| Status | Meaning | Typical Nudm Usage |
|---|---|---|
| `200 OK` | Request succeeded | GET responses, successful updates |
| `201 Created` | Resource created | New subscriptions, registrations |
| `204 No Content` | Success, no body | DELETE operations |
| `307 Temporary Redirect` | Redirect to another NF | NF instance relocation |
| `308 Permanent Redirect` | Permanent redirect | NF instance replacement |
| `400 Bad Request` | Malformed request | Invalid JSON, missing mandatory fields |
| `401 Unauthorized` | Authentication failure | Missing or invalid access token |
| `403 Forbidden` | Authorization failure | Insufficient scope, subscriber not authorized |
| `404 Not Found` | Resource not found | Unknown SUPI, subscription ID not found |
| `405 Method Not Allowed` | Wrong HTTP method | PUT on a POST-only endpoint |
| `406 Not Acceptable` | Cannot satisfy Accept header | Unsupported media type in Accept |
| `409 Conflict` | Resource conflict | Duplicate registration |
| `411 Length Required` | Missing Content-Length | — |
| `413 Payload Too Large` | Request body too large | — |
| `415 Unsupported Media Type` | Wrong Content-Type | Non-JSON body |
| `429 Too Many Requests` | Rate limit exceeded | Consumer exceeding allowed rate |
| `500 Internal Server Error` | Server error | UDR connection failure |
| `501 Not Implemented` | Feature not supported | Optional feature not deployed |
| `502 Bad Gateway` | Upstream error | UDR returned unexpected response |
| `503 Service Unavailable` | Temporarily unavailable | UDM overloaded or in maintenance |
| `504 Gateway Timeout` | Upstream timeout | UDR response timeout |

### 7.3 3GPP Application-Level Cause Codes

The `cause` field in ProblemDetails carries a 3GPP-defined application error string. Common
Nudm cause values include:

| Cause | Meaning | Service |
|---|---|---|
| `AUTHENTICATION_REJECTED` | Auth data generation rejected | UEAU |
| `SERVING_NETWORK_NOT_AUTHORIZED` | Serving network not allowed | UEAU |
| `USER_NOT_FOUND` | Subscriber unknown | All |
| `DATA_NOT_FOUND` | Requested data does not exist | SDM, UECM |
| `CONTEXT_NOT_FOUND` | Registration context not found | UECM |
| `SUBSCRIPTION_NOT_FOUND` | Subscription ID unknown | SDM, EE |
| `MODIFICATION_NOT_ALLOWED` | Resource cannot be modified | SDM, UECM |
| `MANDATORY_IE_INCORRECT` | Mandatory field has wrong value | All |
| `MANDATORY_IE_MISSING` | Required field omitted | All |
| `UNSPECIFIED_NF_FAILURE` | Internal NF error | All |
| `NF_CONGESTION` | NF is congested | All |
| `INSUFFICIENT_RESOURCES` | Cannot allocate resources | All |

### 7.4 Error Response Examples

**404 — Subscriber Not Found:**

```json
{
  "status": 404,
  "title": "Subscriber Not Found",
  "detail": "No subscription data found for the given SUPI.",
  "cause": "USER_NOT_FOUND"
}
```

**429 — Rate Limited:**

```json
{
  "status": 429,
  "title": "Too Many Requests",
  "detail": "Request rate exceeded the configured limit.",
  "cause": "NF_CONGESTION"
}
```

**503 — Service Unavailable with Retry-After:**

```http
HTTP/2 503
Content-Type: application/problem+json
Retry-After: 30
3gpp-Sbi-Oci: ts=2024-06-04T10:30:00Z; validity=60; metric=80

{
  "status": 503,
  "title": "Service Unavailable",
  "detail": "UDM is temporarily overloaded. Retry after the indicated period.",
  "cause": "NF_CONGESTION"
}
```

---

## 8. API Versioning Strategy

### 8.1 URI-Based Versioning

3GPP SBI APIs use **URI path versioning** with a `v{Major}` segment:

```
{apiRoot}/nudm-sdm/v2/{supi}/am-data
                    ^^
                    Major version in URI
```

- **Major version** increments indicate backward-incompatible changes and are reflected in the
  URI path (e.g., `v1` → `v2`).
- **Minor and patch versions** are tracked in the OpenAPI specification metadata but do **not**
  change the URI.

### 8.2 Version Negotiation

| Mechanism | Description |
|---|---|
| **NRF Profile** | Each UDM instance registers its supported API versions in its NF profile. Consumers discover compatible versions via `Nnrf_NFDiscovery`. |
| **Supported Features** | The `supportedFeatures` query parameter and response field (a hex-encoded bitmask) negotiate optional feature sets within a major version. |
| **Backward Compatibility** | Within a major version, producers accept requests from older minor versions. New optional fields are ignored by older consumers. |

### 8.3 Current Nudm API Versions

| API | Major | Full Version | Notes |
|---|---|---|---|
| Nudm\_UEAU | v1 | 1.3.2 | Stable |
| Nudm\_SDM | v2 | 2.3.6 | Upgraded from v1 in Rel-17 |
| Nudm\_UECM | v1 | 1.3.3 | Stable |
| Nudm\_EE | v1 | 1.3.1 | Stable |
| Nudm\_PP | v1 | 1.3.3 | Stable |
| Nudm\_MT | v1 | 1.2.0 | Stable |
| Nudm\_SSAU | v1 | 1.1.1 | Stable |
| Nudm\_NIDDAU | v1 | 1.2.0 | Stable |
| Nudm\_RSDS | v1 | 1.2.0 | Stable |
| Nudm\_UEID | v1 | 1.0.0 | New in Rel-17 |

---

## 9. Content Negotiation

### 9.1 Media Types

| Header | Standard Value | Patch Operations |
|---|---|---|
| `Content-Type` (request) | `application/json` | `application/merge-patch+json` (RFC 7386) |
| `Content-Type` (response) | `application/json` | — |
| `Content-Type` (errors) | `application/problem+json` | — |
| `Accept` (request) | `application/json` | — |

### 9.2 JSON Encoding Rules

Per 3GPP TS 29.500:

- **Character encoding:** UTF-8 (mandatory)
- **Null values:** Fields with null values should be omitted from the JSON body rather than
  sent as `null`, unless explicitly required by the schema
- **Unknown fields:** Consumers must ignore unrecognized JSON fields (forward compatibility)
- **Empty arrays:** May be omitted or sent as `[]` depending on the schema's `minItems`
- **Enumerations:** Consumers must accept unknown enum values gracefully (extensible enums use
  the `anyOf` pattern in OpenAPI)
- **DateTime format:** ISO 8601 / RFC 3339 (`2024-06-04T10:30:00Z`)

### 9.3 Content Encoding

Clients may request compressed responses and indicate accepted notification encodings:

```http
Accept-Encoding: gzip, deflate
3gpp-Sbi-Notif-Accepted-Encoding: gzip;q=1.0, identity;q=0.5
```

### 9.4 Conditional Requests

Several Nudm APIs support conditional requests for caching optimization:

| Header | Direction | Purpose |
|---|---|---|
| `ETag` | Response | Opaque version tag for the returned resource |
| `Last-Modified` | Response | ISO 8601 timestamp of last modification |
| `If-None-Match` | Request | Skip response body if ETag matches (returns 304) |
| `If-Match` | Request | Ensure update targets the expected version (optimistic concurrency) |
| `Cache-Control` | Response | Caching directives with `max-age` |

---

## 10. Pagination

### 10.1 List Operations

Endpoints that return collections (e.g., SMF registrations, shared data) support pagination
using query parameters:

| Parameter | Type | Description |
|---|---|---|
| `limit` | integer | Maximum number of items to return per page |
| `offset` | integer | Number of items to skip (zero-based) |

### 10.2 Example — List SMF Registrations

**Request:**

```http
GET /nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations?limit=10 HTTP/2
Authorization: Bearer eyJhbGciOi...
```

**Response Headers:**

```http
HTTP/2 200 OK
Content-Type: application/json
Link: </nudm-uecm/v1/imsi-001010000000001/registrations/smf-registrations?limit=10&offset=10>; rel="next"
```

### 10.3 Link Header

When more results are available, the response includes a `Link` header (RFC 8288) with
`rel="next"` pointing to the next page. Consumers should follow these links rather than
constructing pagination URLs manually.

### 10.4 Query Filtering

Many GET endpoints support query parameters to filter results at the server side, reducing the
need for pagination:

| Service | Endpoint | Filter Parameters |
|---|---|---|
| SDM | `/{supi}/sm-data` | `snssai`, `dnn`, `plmn-id` |
| SDM | `/{supi}/am-data` | `plmn-id`, `adjacent-plmns`, `disaster-roaming-ind` |
| SDM | `/shared-data` | `shared-data-ids` (array) |
| UECM | `/{ueId}/registrations` | `registration-dataset-names` |
| SDM | `/{supi}/nssai` | `plmn-id`, `disaster-roaming-ind` |

---

## 11. Rate Limiting and Overload Control

### 11.1 3GPP Overload Control (OCI)

When a UDM instance becomes overloaded, it signals consumers using the `3gpp-Sbi-Oci` header:

```http
HTTP/2 200 OK
3gpp-Sbi-Oci: ts=2024-06-04T10:30:00Z; validity=120; metric=60
```

| Field | Meaning |
|---|---|
| `ts` | Timestamp when overload was detected |
| `validity` | Duration (seconds) the OCI remains valid |
| `metric` | Overload reduction percentage (0–100). Value of 60 means reduce traffic by 60%. |
| `scope` | Optional scope limiting the OCI to specific services or operations |

**Consumer behavior upon receiving OCI:**

1. Reduce request rate to the indicated producer by the `metric` percentage
2. Re-route excess traffic to alternative NF instances (discovered via NRF)
3. Honor the `validity` period — re-evaluate when it expires
4. If `scope` is present, apply reduction only to the indicated services

### 11.2 Load Control (LCI)

Producers proactively advertise their load via the `3gpp-Sbi-Lci` header:

```http
HTTP/2 200 OK
3gpp-Sbi-Lci: ts=2024-06-04T10:30:00Z; metric=75
```

| Field | Meaning |
|---|---|
| `ts` | Timestamp of the load measurement |
| `metric` | Current load level (0–100). 0 = idle, 100 = fully loaded. |
| `scope` | Optional scope for the load indication |

Consumers and SCPs use LCI for **load-aware routing** — preferring NF instances with lower
load metrics when multiple instances are available.

### 11.3 HTTP 429 and Retry-After

When rate limiting is enforced at the HTTP level:

```http
HTTP/2 429 Too Many Requests
Content-Type: application/problem+json
Retry-After: 5

{
  "status": 429,
  "title": "Too Many Requests",
  "cause": "NF_CONGESTION"
}
```

Consumers must:

1. Stop sending requests to that producer for the `Retry-After` duration
2. Apply exponential backoff if repeated 429s are received
3. Consult NRF for alternative producer instances

### 11.4 Backpressure via SCP

In deployments using an SCP (Service Communication Proxy), the SCP acts as a central point
for load balancing and overload management:

```
Consumer NF ──▶ SCP ──▶ UDM Instance 1 (load: 80)
                   ├──▶ UDM Instance 2 (load: 30)  ← preferred
                   └──▶ UDM Instance 3 (load: 95, OCI active)  ← avoided
```

The SCP aggregates LCI and OCI signals from all UDM instances and makes routing decisions
on behalf of consumer NFs, providing transparent load distribution.

### 11.5 Retry and Selection Headers

Retry behavior is further controlled by dedicated headers:

| Header | Purpose |
|---|---|
| `3gpp-Sbi-Retry-Info: no-retries` | Producer signals that the consumer should not retry the request |
| `3gpp-Sbi-Request-Info: retrans=true` | Consumer indicates this is a retransmission of a previous request |
| `3gpp-Sbi-Request-Info: idempotency-key=<key>` | Idempotency key ensuring at-most-once processing of retransmissions |
| `3gpp-Sbi-Selection-Info: reselection=true` | Consumer requests the SCP to reselect a different NF instance |

---

## References

- **3GPP TS 29.500** — 5G System; Technical Realization of Service Based Architecture
- **3GPP TS 29.501** — 5G System; Principles and Guidelines for Services Definition
- **3GPP TS 29.503** — 5G System; Unified Data Management Services (all Nudm APIs)
- **3GPP TS 29.505** — 5G System; Usage of the Unified Data Repository Services
- **3GPP TS 33.501** — Security Architecture and Procedures for 5G System
- **IETF RFC 7807** — Problem Details for HTTP APIs
- **IETF RFC 7386** — JSON Merge Patch
- **IETF RFC 6749** — The OAuth 2.0 Authorization Framework
- **IETF RFC 8259** — The JavaScript Object Notation (JSON) Data Interchange Format
- **IETF RFC 8288** — Web Linking
- **`docs/3gpp/TS29500_CustomHeaders.abnf`** — 3GPP custom header ABNF definitions (v18.6.1)

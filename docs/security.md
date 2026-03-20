# UDM Security Architecture

| Document Property | Value |
|-------------------|-------|
| **Version** | 1.0.0 |
| **Status** | Draft |
| **Classification** | Internal — Engineering |
| **Last Updated** | 2025 |
| **Parent Document** | [architecture.md](architecture.md) |

---

## Table of Contents

1. [Security Overview](#1-security-overview)
2. [Authentication and Authorization](#2-authentication-and-authorization)
3. [Transport Security](#3-transport-security)
4. [Subscriber Identity Protection](#4-subscriber-identity-protection)
5. [Authentication Credential Security](#5-authentication-credential-security)
6. [Data Security](#6-data-security)
7. [Network Security](#7-network-security)
8. [Secure Development Practices](#8-secure-development-practices)
9. [Incident Response](#9-incident-response)
10. [Compliance Matrix](#10-compliance-matrix)

---

## 1. Security Overview

### 1.1 Purpose

This document defines the security architecture for the 5G Unified Data Management (UDM) system. The UDM is the most security-critical network function in the 5G core: it stores permanent subscriber authentication keys (K, OPc), manages subscriber identities (SUPI/SUCI), and generates authentication vectors for every subscriber attach. A breach of the UDM directly compromises the confidentiality and integrity of all subscribers on the network.

All security controls in this document are designed to comply with **3GPP TS 33.501** ("Security architecture and procedures for 5G System"), referenced at [`docs/3gpp/33501-ia0.docx`](3gpp/33501-ia0.docx).

### 1.2 Threat Model

The UDM faces threats from multiple adversary classes:

| Threat Actor | Capability | Primary Target |
|---|---|---|
| **External attacker** | Network-level access to SBI interfaces | Authentication credentials, subscriber data |
| **Compromised NF** | Valid OAuth2 token with limited scope | Lateral movement, privilege escalation |
| **Insider (operator staff)** | Database or Kubernetes admin access | Bulk subscriber data exfiltration |
| **Supply chain** | Malicious dependency or container image | Code execution within UDM pods |
| **State-level adversary** | Passive interception of air interface | SUPI correlation, subscriber tracking |

### 1.3 Defense-in-Depth Strategy

The UDM employs layered security controls so that no single failure compromises the system:

```
┌─────────────────────────────────────────────────────────────────┐
│  Layer 1: Network Perimeter — Ingress controls, DDoS mitigation │
├─────────────────────────────────────────────────────────────────┤
│  Layer 2: Transport — TLS 1.3, mTLS for all SBI interfaces      │
├─────────────────────────────────────────────────────────────────┤
│  Layer 3: Authentication — OAuth2 + NF identity verification     │
├─────────────────────────────────────────────────────────────────┤
│  Layer 4: Authorization — Per-service scopes, RBAC               │
├─────────────────────────────────────────────────────────────────┤
│  Layer 5: Application — Input validation, secure coding          │
├─────────────────────────────────────────────────────────────────┤
│  Layer 6: Data — Encryption at rest, column-level encryption     │
├─────────────────────────────────────────────────────────────────┤
│  Layer 7: Audit — Comprehensive logging, SIEM integration        │
└─────────────────────────────────────────────────────────────────┘
```

### 1.4 Zero-Trust Architecture

The UDM operates under zero-trust principles — no implicit trust is granted based on network location:

- **Verify explicitly** — Every request is authenticated (mTLS + OAuth2 token) and authorized (scope check) regardless of source.
- **Least privilege** — Each NF consumer receives only the OAuth2 scopes required for its function.
- **Assume breach** — All internal traffic is encrypted; audit logs capture every data access for forensic analysis.
- **Micro-segmentation** — Kubernetes network policies restrict pod-to-pod communication to explicitly allowed paths.

---

## 2. Authentication and Authorization

### 2.1 OAuth2 Client Credentials Flow

All NF-to-NF communication on the Service-Based Interface (SBI) uses the **OAuth2 Client Credentials** grant as specified in 3GPP TS 33.501 §13.3. The NRF acts as the OAuth2 authorization server.

**Token acquisition flow:**

```
┌───────────┐         ┌───────────┐         ┌───────────┐
│ NF Consumer│         │    NRF    │         │    UDM    │
│  (e.g. AMF)│         │(AuthZ Srv)│         │(Resource) │
└─────┬─────┘         └─────┬─────┘         └─────┬─────┘
      │  1. POST /oauth2/token    │                │
      │  (client_id, client_secret│                │
      │   scope=nudm-ueau)        │                │
      ├──────────────────────────►│                │
      │                           │                │
      │  2. 200 OK               │                │
      │  { access_token, expires_in }              │
      │◄──────────────────────────┤                │
      │                           │                │
      │  3. GET /nudm-ueau/v1/...│                │
      │  Authorization: Bearer <token>             │
      ├────────────────────────────────────────────►
      │                           │                │
      │  4. Validate token        │                │
      │     (signature, scope,    │                │
      │      expiry, NF type)     │                │
      │                           │  5. 200 OK     │
      │◄────────────────────────────────────────────
```

### 2.2 Per-Service OAuth2 Scopes

Each of the 10 Nudm microservices defines its own OAuth2 scope. Tokens are issued with the minimum set of scopes required by the consuming NF:

| Nudm Service | OAuth2 Scope | Authorized Consumers |
|---|---|---|
| Subscriber Data Management | `nudm-sdm` | AMF, SMF, SMSF |
| UE Authentication | `nudm-ueau` | AUSF |
| UE Context Management | `nudm-uecm` | AMF, SMF, SMSF |
| Event Exposure | `nudm-ee` | NEF, AMF |
| Parameter Provision | `nudm-pp` | NEF, PCF |
| NIDD Authorization | `nudm-niddau` | NEF |
| MT (Mobile Terminated) | `nudm-mt` | SMS-GMSC |
| Session Management Subscription | `nudm-sdm-sub` | SMF |
| Reporting | `nudm-report` | OAM |
| SUCI Deconceal | `nudm-sdec` | AUSF, SIDF |

### 2.3 NF Consumer Identity Verification

Beyond token validation, the UDM verifies that the calling NF's identity is consistent across all layers:

1. **mTLS certificate** — The client certificate Subject Alternative Name (SAN) must contain the NF instance ID.
2. **OAuth2 token claims** — The `nfInstanceId` and `nfType` claims in the access token must match the mTLS certificate SAN.
3. **NRF profile cross-check** — On first contact, the UDM queries the NRF to verify the NF instance is registered and its claimed type is correct.

If any of these checks fail, the request is rejected with `403 Forbidden` and a security event is logged.

### 2.4 Token Validation and Caching

Token validation is performed on every request. To avoid per-request calls to the NRF:

- **Local validation** — Tokens are signed JWTs (RS256 or ES256). The UDM validates the signature locally using the NRF's cached public key.
- **Key refresh** — The NRF JWKS endpoint is polled every 5 minutes. On signature validation failure, an immediate refresh is triggered.
- **Token cache** — Validated tokens are cached in-memory (Go `sync.Map`) for the remaining TTL, keyed by token hash. Maximum cache size is bounded to prevent memory exhaustion.
- **Clock skew tolerance** — A maximum clock skew of 30 seconds is allowed for `exp` and `nbf` claims.

### 2.5 mTLS for SBI Interfaces

All SBI communication uses mutual TLS. Both the client and server present X.509 certificates during the TLS handshake:

| Property | Value |
|---|---|
| **TLS version** | 1.3 (mandatory), 1.2 rejected |
| **Client certificate required** | Yes — connection refused without valid client cert |
| **Certificate authority** | Operator-managed private PKI (not public CAs) |
| **SAN format** | `<nfInstanceId>.5gc.mnc<MNC>.mcc<MCC>.3gppnetwork.org` |
| **Revocation check** | OCSP stapling (primary), CRL (fallback) |

### 2.6 Service Mesh mTLS Enforcement

Istio service mesh provides a secondary layer of mTLS enforcement within the Kubernetes cluster:

- **PeerAuthentication** — Set to `STRICT` mode for the UDM namespace. All pod-to-pod traffic must be mTLS; plaintext connections are rejected.
- **AuthorizationPolicy** — Istio authorization policies restrict which services can call each UDM microservice, mirroring the OAuth2 scope table.
- **Certificate rotation** — Istio's Citadel rotates workload certificates every 24 hours automatically.
- **SPIFFE identity** — Each pod receives a SPIFFE identity (`spiffe://cluster.local/ns/udm/sa/<service-account>`) embedded in the workload certificate.

---

## 3. Transport Security

### 3.1 TLS 1.3 Mandate

All interfaces — SBI, database connections, observability exporters — use TLS 1.3 as the minimum version. TLS 1.2 and below are disabled at the application layer.

| Interface | Protocol | TLS Termination |
|---|---|---|
| NF-to-UDM (SBI) | HTTP/2 over TLS 1.3 | Application-level (Go `crypto/tls`) |
| UDM-to-YugabyteDB | PostgreSQL wire protocol over TLS 1.3 | Client-side + server-side |
| UDM-to-NRF | HTTP/2 over TLS 1.3 | Application-level |
| Observability (metrics) | HTTPS | Sidecar (Istio) |
| Admin/OAM | HTTPS | Ingress controller |

### 3.2 Certificate Management (PKI)

The UDM uses a two-tier PKI hierarchy operated by the network operator:

```
Root CA (offline, air-gapped)
  └── Intermediate CA (online, automated)
        ├── NF server certificates
        ├── NF client certificates
        └── Database server/client certificates
```

- **Root CA** — Stored in an air-gapped HSM. Used only to sign intermediate CA certificates. Validity: 10 years.
- **Intermediate CA** — Managed by cert-manager in Kubernetes. Issues short-lived certificates. Validity: 2 years.
- **Leaf certificates** — Issued to each UDM pod. Validity: 90 days.

### 3.3 Certificate Rotation Strategy

Certificates are rotated automatically before expiry with zero downtime:

| Component | Rotation Period | Mechanism |
|---|---|---|
| Istio workload certs | 24 hours | Citadel automatic rotation |
| NF SBI certificates | 30 days | cert-manager `Certificate` CRD with 2/3-life renewal |
| YugabyteDB server certs | 90 days | cert-manager + rolling restart via StatefulSet |
| NRF JWKS signing key | 90 days | NRF key rotation with overlapping validity |
| Intermediate CA | 1 year | Manual rotation with planned maintenance window |

During rotation, both old and new certificates are valid simultaneously (overlapping validity windows) to prevent connection failures.

### 3.4 Cipher Suite Restrictions

Only AEAD cipher suites with forward secrecy are permitted:

```
# TLS 1.3 cipher suites (all provide forward secrecy)
TLS_AES_256_GCM_SHA384
TLS_AES_128_GCM_SHA256
TLS_CHACHA20_POLY1305_SHA256

# Explicitly disabled
TLS_AES_128_CCM_SHA256       # Not widely supported
TLS_AES_128_CCM_8_SHA256     # Reduced tag length
```

The Go `crypto/tls` configuration:

```go
tlsConfig := &tls.Config{
    MinVersion: tls.VersionTLS13,
    CipherSuites: nil, // Go 1.21+ uses secure defaults for TLS 1.3
    CurvePreferences: []tls.CurveID{
        tls.X25519,
        tls.CurveP256,
    },
}
```

### 3.5 HTTP/2 Security Considerations

All SBI interfaces use HTTP/2 over TLS 1.3. The following HTTP/2-specific protections are applied:

- **SETTINGS_MAX_CONCURRENT_STREAMS** — Limited to 250 per connection to prevent stream exhaustion.
- **SETTINGS_MAX_HEADER_LIST_SIZE** — Limited to 64 KB to prevent header compression attacks (HPACK bomb).
- **Flow control** — Default flow control windows are used; no custom enlargement.
- **Connection idle timeout** — 120 seconds. Idle connections are closed to free resources.
- **PING keepalive** — Sent every 30 seconds to detect dead connections.

---

## 4. Subscriber Identity Protection

### 4.1 SUPI and SUCI Overview (TS 33.501 §6.12)

Per 3GPP TS 33.501 §6.12, the Subscription Permanent Identifier (SUPI) must never be transmitted in cleartext over the air interface. The UE encrypts the SUPI into a Subscription Concealed Identifier (SUCI) before sending it to the network.

| Identifier | Format | Confidentiality |
|---|---|---|
| **SUPI** | `imsi-<MCC><MNC><MSIN>` or `nai-<user@realm>` | Never sent over air interface; protected within core |
| **SUCI** | `suci-0-<MCC>-<MNC>-<routing_id>-<scheme_id>-<HN_pub_key_id>-<encrypted_MSIN>` | Sent by UE; contains encrypted MSIN portion |
| **GPSI** | `msisdn-<E.164>` or `extid-<external_id>` | Used for external-facing interactions; mapped from SUPI |

### 4.2 SUCI Encryption Scheme (ECIES Profile A/B)

The UDM supports both ECIES profiles defined in TS 33.501 Annex C:

| Property | Profile A | Profile B |
|---|---|---|
| **Elliptic curve** | X25519 | secp256r1 (P-256) |
| **KDF** | ANSI-X9.63-KDF with SHA-256 | ANSI-X9.63-KDF with SHA-256 |
| **Encryption** | AES-128-CTR | AES-128-CTR |
| **MAC** | HMAC-SHA-256 (truncated to 64 bits) | HMAC-SHA-256 (truncated to 64 bits) |
| **Ephemeral key size** | 32 bytes (X25519 public key) | 33 bytes (compressed P-256 point) |

The UDM is configured to prefer **Profile A** (X25519) for new deployments due to superior performance and resistance to timing side-channel attacks.

### 4.3 Home Network Key Management

The SUCI deconceal operation requires the home network's private key. Key management follows strict controls:

- **Key generation** — Private keys are generated inside an HSM (FIPS 140-2 Level 3 or higher). The private key never leaves the HSM in plaintext.
- **Key storage** — The HSM stores the private key indexed by `HN_pub_key_id`. Multiple key pairs exist simultaneously to support rotation.
- **Public key distribution** — The home network public key and `HN_pub_key_id` are provisioned to UE SIM cards during manufacture or via OTA update.
- **Key rotation** — New key pairs are generated annually. Old keys remain active for the lifetime of SIMs provisioned with them (up to 10 years).
- **Key backup** — HSM key material is backed up to a secondary HSM in a geographically separate facility using vendor-specific secure key export.

### 4.4 SUCI Deconceal Process Security

The SUCI-to-SUPI deconceal operation is the most sensitive cryptographic operation in the UDM. It is performed by the Subscription Identifier De-concealing Function (SIDF), which runs as part of the `nudm-sdec` microservice:

1. The AUSF forwards the SUCI to the UDM (`nudm-sdec`).
2. The UDM extracts the `scheme_id` and `HN_pub_key_id` from the SUCI.
3. The UDM passes the encrypted MSIN and the ephemeral public key to the HSM.
4. The HSM performs the ECIES decryption using the home network private key and returns the plaintext MSIN.
5. The UDM reconstructs the SUPI from the decrypted MSIN and the MCC/MNC.

**Security invariants:**

- The home network private key is **never** loaded into application memory — all decryption is performed within the HSM boundary.
- The deconceal operation is rate-limited to 50,000 operations/second per HSM to prevent denial-of-service.
- Failed deconceal attempts (invalid SUCI, unknown key ID) are logged as security events with the source NF identity.

### 4.5 SUPI Protection Within the Core

Although the SUPI is transmitted between core NFs (e.g., UDM → AMF), it is always protected by mTLS and restricted by authorization policy:

- SUPI is included in SBI responses only when the consuming NF's OAuth2 scope permits it.
- SUPI is logged only in redacted form (`imsi-***<last 4 digits>`) in application logs.
- Full SUPI appears only in audit logs stored in a separate, access-controlled logging pipeline.

### 4.6 GPSI Handling

The Generic Public Subscription Identifier (GPSI) maps to external-facing identifiers (typically MSISDN). The UDM enforces:

- GPSI-to-SUPI mapping is available only to NFs with the `nudm-sdm` scope.
- GPSI values are treated as PII and subject to the same data protection controls as SUPI.
- Reverse mapping (SUPI → GPSI) is restricted to authorized network functions (AMF, SMF, NEF).

---

## 5. Authentication Credential Security

### 5.1 Permanent Key (K) Storage

The permanent authentication key (K) is a 128-bit symmetric key shared between the UE's USIM and the UDM. Compromise of K allows an attacker to impersonate the subscriber or decrypt their traffic.

**Protection measures:**

| Control | Implementation |
|---|---|
| **Storage** | K is stored in YugabyteDB with column-level AES-256-GCM encryption |
| **Encryption key** | The column encryption key (CEK) is wrapped by a key-encryption key (KEK) stored in the HSM |
| **Access** | Only the `nudm-ueau` microservice has database credentials to read the K column |
| **In-memory** | K is held in memory only for the duration of a single authentication vector generation (~2 ms) and then zeroed |
| **Audit** | Every read of K is logged with the requesting SUPI, source NF, and timestamp |
| **Bulk export** | No API or tooling exists to export K values in bulk. Database `SELECT *` on the credentials table returns encrypted ciphertext |

### 5.2 Operator Key (OPc) Handling

OPc is derived from the operator-specific key (OP) and the subscriber's permanent key (K) using the Milenage algorithm: `OPc = AES-128(K, OP) ⊕ OP`.

- **Pre-computation** — OPc is pre-computed and stored alongside K to avoid storing the global OP value in the UDM database.
- **Same protections as K** — OPc is column-level encrypted with the same CEK and access controls.
- **OP is never stored** — The operator key OP is used only during initial OPc derivation (in a secure provisioning environment) and is not present in the UDM system.

### 5.3 Sequence Number (SQN) Management

The SQN prevents replay attacks in AKA authentication. The UDM maintains a per-subscriber SQN counter:

- **Storage** — SQN is stored in YugabyteDB as a 48-bit integer, updated atomically after each authentication vector generation.
- **Concurrency** — YugabyteDB's distributed transactions ensure SQN updates are serializable even in multi-region active-active deployments.
- **SQN resynchronization** — When the UE detects an SQN mismatch (MAC-S failure), it sends an AUTS token. The UDM verifies AUTS using the subscriber's K and resyncs the SQN. Resync events are logged as security events because they can indicate cloning attacks.
- **SQN window** — The UDM implements the SQN verification mechanism from TS 33.102 §C.2 with a window size of 32 to tolerate interleaved authentication requests.

### 5.4 Authentication Vector Generation

The UDM generates 5G authentication vectors (5G HE AV) for the AUSF using the 5G-AKA protocol:

1. Retrieve K, OPc, and SQN for the requested SUPI (decrypting from column-level encryption).
2. Generate a 128-bit random number (RAND) using `crypto/rand` (OS-level CSPRNG).
3. Compute Milenage functions (f1–f5, f1\*, f5\*) to produce XRES, CK, IK, AK, AUTN.
4. Derive KAUSF using the 3GPP KDF: `KAUSF = KDF(CK||IK, serving_network_name)`.
5. Compute XRES\* = KDF(CK||IK, serving_network_name, RAND, XRES).
6. Compute HXRES\* = SHA-256(RAND || XRES\*) for the AUSF.
7. Increment SQN and persist to database.
8. Return 5G HE AV = (RAND, AUTN, HXRES\*, KAUSF) to AUSF.
9. Zero all intermediate key material (CK, IK, XRES, K, OPc) from memory.

### 5.5 Key Derivation Functions

All key derivations follow the 3GPP KDF specified in TS 33.220 Annex B:

```
KDF(Key, FC, P0, L0, P1, L1, ...) = HMAC-SHA-256(Key, S)
where S = FC || P0 || L0 || P1 || L1 || ...
```

The Go implementation uses `crypto/hmac` with `crypto/sha256` and is validated against the 3GPP test vectors in TS 33.501 Annex A.

### 5.6 Hardware Security Module (HSM) Integration

The UDM integrates with FIPS 140-2 Level 3 (or higher) HSMs for all critical key operations:

| Operation | HSM Role |
|---|---|
| SUCI deconceal (ECIES decryption) | Private key stored in HSM; decryption performed inside HSM |
| Column encryption KEK storage | KEK stored in HSM; CEK wrapping/unwrapping via HSM API |
| Authentication vector generation | Optional — Milenage computation can be offloaded to HSM for high-security deployments |
| Random number generation | HSM TRNG supplements OS CSPRNG for RAND generation |

**HSM connectivity:**

- The UDM connects to the HSM via PKCS#11 interface over a dedicated, non-routable management network.
- HSM sessions are pooled (max 64 concurrent sessions) to avoid connection setup overhead.
- HSM health is monitored; if the HSM becomes unreachable, authentication requests are rejected (fail-closed) rather than falling back to software-based operations.

---

## 6. Data Security

### 6.1 Encryption at Rest

YugabyteDB provides transparent encryption at rest for all data files:

| Property | Value |
|---|---|
| **Algorithm** | AES-256-CTR for data files |
| **Key management** | Universe key stored in HashiCorp Vault (backed by HSM) |
| **Scope** | All SSTables, WAL files, and temporary files |
| **Key rotation** | Universe key rotated every 90 days; triggers background re-encryption of SSTables |

### 6.2 Encryption in Transit (Database)

All connections between UDM microservices and YugabyteDB use TLS 1.3:

- YugabyteDB TServer and Master nodes present server certificates.
- UDM microservices present client certificates (mTLS).
- The Go `pgx` driver is configured with `sslmode=verify-full` to validate server certificate hostname.

### 6.3 Column-Level Encryption

Transparent encryption at rest protects against storage-level attacks but not against compromised application connections. For the most sensitive fields, column-level encryption adds an additional layer:

| Table | Column | Encryption | CEK Rotation |
|---|---|---|---|
| `auth_credentials` | `k_key` | AES-256-GCM | 180 days |
| `auth_credentials` | `opc_key` | AES-256-GCM | 180 days |
| `subscriber_identity` | `supi` | AES-256-GCM | 365 days |
| `subscriber_identity` | `gpsi` | AES-256-GCM | 365 days |

Column-level encryption is implemented in the Go application layer using envelope encryption:

1. A unique data encryption key (DEK) encrypts the column value.
2. The DEK is wrapped (encrypted) by a KEK stored in the HSM.
3. The wrapped DEK is stored alongside the ciphertext in the database.
4. On read, the wrapped DEK is sent to the HSM for unwrapping, then used to decrypt the column.

### 6.4 Data Classification

All data handled by the UDM is classified into tiers with corresponding protection requirements:

| Classification | Examples | Protection |
|---|---|---|
| **Critical** | K, OPc, home network private keys | HSM-protected, column-encrypted, strict access control, full audit |
| **Highly Sensitive** | SUPI, GPSI, authentication vectors | Column-encrypted, scoped access, audit logged |
| **Sensitive** | Registration state, session context, subscriber profiles | Encrypted at rest, scoped access |
| **Internal** | Service configuration, routing data, metrics | Encrypted at rest, standard access controls |

### 6.5 Data Retention Policies

| Data Category | Retention Period | Deletion Method |
|---|---|---|
| Authentication credentials (K, OPc) | Lifetime of subscription | Cryptographic erasure on deprovisioning |
| Authentication event logs | 12 months | Automatic purge via TTL |
| Subscriber profile data | Lifetime of subscription + 30 days | Secure deletion on deprovisioning |
| SQN counters | Lifetime of subscription | Deleted with credentials |
| Audit logs | 24 months (regulatory minimum) | Immutable storage, lifecycle policy |
| Observability metrics | 90 days | Automatic rollover |

### 6.6 GDPR and Privacy Compliance

The UDM processes personal data (SUPI, GPSI, location-related registration state) subject to GDPR and regional privacy regulations:

- **Data minimization** — Each microservice accesses only the subscriber data fields required for its function.
- **Purpose limitation** — OAuth2 scopes enforce that subscriber data is used only for the authorized network function purpose.
- **Right to erasure** — The deprovisioning API triggers cryptographic erasure of all subscriber data, including K/OPc (by destroying the per-subscriber DEK).
- **Data portability** — Subscriber profile data can be exported in 3GPP-standard formats via the OAM interface.
- **Privacy by design** — SUPI redaction in logs, SUCI concealment on air interface, and column-level encryption are built into the architecture.

### 6.7 Audit Logging

Every access to subscriber data is logged in an immutable audit trail:

```json
{
  "timestamp": "2025-01-15T10:30:00.000Z",
  "event_type": "DATA_ACCESS",
  "service": "nudm-ueau",
  "operation": "GET_AUTH_VECTORS",
  "supi_redacted": "imsi-***1234",
  "source_nf_type": "AUSF",
  "source_nf_instance": "ausf-01.5gc.mnc001.mcc001.3gppnetwork.org",
  "oauth2_scope": "nudm-ueau",
  "result": "SUCCESS",
  "fields_accessed": ["k_key", "opc_key", "sqn"],
  "trace_id": "abc123def456"
}
```

Audit logs are:
- Written to a dedicated Kafka topic (`udm.audit.log`) separate from application logs.
- Stored in append-only, tamper-evident storage (write-once object storage with integrity checksums).
- Retained for a minimum of 24 months.
- Accessible only to the security operations team (Kubernetes RBAC + separate credentials).

---

## 7. Network Security

### 7.1 Network Segmentation

The UDM deployment spans three isolated network planes:

| Plane | Purpose | Networks |
|---|---|---|
| **Signaling (SBI)** | NF-to-NF communication | Kubernetes ClusterIP services, Istio mTLS |
| **Data (DB)** | UDM-to-YugabyteDB communication | Dedicated subnet, no external routing |
| **Management (OAM)** | Operations, monitoring, admin access | Separate VLAN, bastion host access only |

Each plane uses a distinct Kubernetes network namespace and CIDR range. No cross-plane traffic is permitted except through explicitly defined network policies.

### 7.2 Kubernetes Network Policies

Network policies enforce allow-list-based pod communication:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: udm-ueau-ingress
  namespace: udm
spec:
  podSelector:
    matchLabels:
      app: nudm-ueau
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              network-zone: sbi
          podSelector:
            matchLabels:
              nf-type: ausf
      ports:
        - protocol: TCP
          port: 8443
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              network-zone: data
          podSelector:
            matchLabels:
              app: yugabytedb
      ports:
        - protocol: TCP
          port: 5433
    - to:
        - namespaceSelector:
            matchLabels:
              network-zone: sbi
          podSelector:
            matchLabels:
              nf-type: nrf
      ports:
        - protocol: TCP
          port: 8443
```

Every Nudm microservice has a corresponding `NetworkPolicy` that limits ingress to its authorized NF consumers and limits egress to YugabyteDB and the NRF.

### 7.3 Pod Security Standards

All UDM pods run with restricted security contexts:

| Control | Setting |
|---|---|
| **runAsNonRoot** | `true` — containers run as UID 65534 (nobody) |
| **readOnlyRootFilesystem** | `true` — no writes to container filesystem |
| **allowPrivilegeEscalation** | `false` |
| **capabilities** | All dropped (`drop: ["ALL"]`) |
| **seccompProfile** | `RuntimeDefault` |
| **Container image** | Distroless base (`gcr.io/distroless/static-debian12`) — no shell, no package manager |
| **Resource limits** | CPU and memory limits enforced to prevent resource exhaustion |

### 7.4 Ingress and Egress Controls

- **Ingress** — SBI traffic enters through an Istio ingress gateway with TLS termination and rate limiting. Only registered NFs with valid mTLS certificates are permitted.
- **Egress** — All outbound traffic is routed through an Istio egress gateway. Only connections to NRF, NSSF, and DNS are permitted. Direct internet access is blocked.
- **DNS** — CoreDNS is configured with a restricted set of upstream resolvers. DNS-over-TLS is used for external resolution.

### 7.5 DDoS Protection

The UDM is protected against volumetric and application-layer DDoS attacks:

- **Layer 3/4** — Cloud provider DDoS protection (e.g., AWS Shield, GCP Cloud Armor) absorbs volumetric attacks at the network edge.
- **Layer 7** — Istio rate limiting and circuit breakers protect individual microservices.
- **Connection limits** — Each UDM microservice limits concurrent connections per source NF instance (default: 500).
- **Request rate limiting** — Token bucket rate limiter per NF consumer, configured per service.

### 7.6 Rate Limiting per NF Consumer

Rate limits are enforced at the Istio ingress gateway and at the application layer:

| Service | Default Rate Limit (req/sec) | Burst |
|---|---|---|
| `nudm-ueau` | 10,000 | 15,000 |
| `nudm-sdm` | 20,000 | 30,000 |
| `nudm-uecm` | 15,000 | 22,500 |
| `nudm-ee` | 5,000 | 7,500 |
| `nudm-sdec` | 50,000 | 50,000 |
| All others | 5,000 | 7,500 |

When a consumer exceeds its rate limit, the UDM returns `429 Too Many Requests` with a `Retry-After` header. Persistent rate limit violations trigger a security alert.

---

## 8. Secure Development Practices

### 8.1 SAST and DAST Integration

Static and dynamic analysis tools are integrated into the CI/CD pipeline:

| Tool | Type | Stage | Purpose |
|---|---|---|---|
| **golangci-lint** | SAST | Pre-commit, CI | Go-specific linting (gosec, staticcheck, errcheck) |
| **gosec** | SAST | CI | Go security-specific static analysis |
| **Semgrep** | SAST | CI | Custom rules for 3GPP-specific security patterns |
| **CodeQL** | SAST | CI (PR gate) | Deep semantic analysis for Go vulnerabilities |
| **OWASP ZAP** | DAST | Staging | Dynamic testing of SBI API endpoints |
| **Trivy** | SCA | CI | Dependency vulnerability scanning |

All SAST findings of severity `HIGH` or `CRITICAL` block the CI pipeline. No code merges to `main` without a clean security scan.

### 8.2 Dependency Vulnerability Scanning

Go module dependencies are continuously monitored:

- **Trivy** scans `go.sum` in every CI run for known CVEs.
- **Dependabot** (or Renovate) creates automated PRs for dependency updates.
- **Go vulnerability database** — `govulncheck` is run weekly against the full dependency tree.
- **Vendor policy** — Critical CVEs in dependencies must be patched or mitigated within 72 hours.

### 8.3 Container Image Scanning

All container images are scanned before deployment:

- **Build-time** — Trivy scans the image in CI before pushing to the registry.
- **Registry** — The container registry (e.g., Harbor) runs continuous scanning on stored images.
- **Runtime** — Admission controllers (OPA Gatekeeper) reject pods using images with unpatched critical CVEs.
- **Base image** — Distroless base images minimize the attack surface (no shell, no package manager, no libc in static variant).
- **Image signing** — All images are signed using Cosign (Sigstore). Kubernetes admission policy validates signatures before scheduling pods.

### 8.4 Go-Specific Security Practices

The UDM codebase enforces Go-specific security standards:

- **No `unsafe` package** — The `unsafe` package is banned. The `gosec` linter rule `G103` and a CI regex check enforce this.
- **No CGo** — CGo is disabled (`CGO_ENABLED=0`) to produce static binaries and eliminate C memory safety issues (exception: PKCS#11 HSM bindings, which are isolated in a dedicated sidecar).
- **Input validation** — All SBI request payloads are validated against the 3GPP OpenAPI schemas using `oapi-codegen`-generated validators before reaching business logic.
- **Integer overflow** — Checked arithmetic is used for SQN counter increments.
- **Cryptography** — Only Go standard library `crypto/*` packages are used. No third-party crypto libraries.
- **Error handling** — Errors are handled explicitly. `errcheck` linter enforces that no errors are silently discarded. Sensitive details are never included in error responses returned to clients.
- **Context propagation** — All operations use `context.Context` with timeouts to prevent resource leaks.

### 8.5 OWASP Compliance

The UDM SBI APIs are secured against OWASP API Security Top 10:

| OWASP Risk | Mitigation |
|---|---|
| **API1: Broken Object Level Authorization** | OAuth2 scopes restrict access per NF type; SUPI-level authorization checks |
| **API2: Broken Authentication** | mTLS + OAuth2 + NF identity cross-check |
| **API3: Broken Object Property Level Authorization** | Response field filtering based on caller's scope |
| **API4: Unrestricted Resource Consumption** | Rate limiting per NF consumer, request size limits |
| **API5: Broken Function Level Authorization** | Per-endpoint scope requirements enforced by middleware |
| **API6: Unrestricted Access to Sensitive Business Flows** | Auth vector generation rate-limited per SUPI |
| **API7: Server Side Request Forgery** | No user-controlled URL parameters; egress restricted |
| **API8: Security Misconfiguration** | Hardened defaults, no debug endpoints in production |
| **API9: Improper Inventory Management** | API versioning, deprecated endpoint monitoring |
| **API10: Unsafe Consumption of APIs** | NRF response validation, strict JSON parsing |

---

## 9. Incident Response

### 9.1 Security Event Logging

The UDM generates security events for the following conditions:

| Event Category | Examples | Severity |
|---|---|---|
| **Authentication failure** | Invalid token, expired token, scope mismatch | WARNING |
| **Authorization violation** | NF accessing unauthorized SUPI, scope escalation attempt | HIGH |
| **Credential access anomaly** | Bulk K/OPc reads, unusual SQN resync rate | CRITICAL |
| **Identity attack** | SUCI deconceal failure, unknown HN_pub_key_id | HIGH |
| **Infrastructure** | HSM unreachable, certificate expiry imminent, TLS handshake failure | HIGH |
| **Rate limit breach** | Sustained rate limit violations by a single NF | WARNING |

### 9.2 SIEM Integration

Security events are exported to the operator's SIEM platform:

```
UDM Pods → Fluentd (sidecar) → Kafka (udm.security.events)
    → SIEM (Splunk / Elastic Security / Sentinel)
    → Alerting (PagerDuty / OpsGenie)
```

- Events are structured in JSON with CEF (Common Event Format) fields for SIEM compatibility.
- The Kafka topic uses a dedicated consumer group with at-least-once delivery guarantees.
- SIEM correlation rules detect cross-NF attack patterns (e.g., an NF probing multiple SUPIs in rapid succession).

### 9.3 Threat Detection

Automated threat detection rules are deployed in the SIEM:

| Rule | Trigger | Response |
|---|---|---|
| **Brute-force SUCI deconceal** | > 100 failed deconceal attempts from a single NF in 60 seconds | Block NF, alert SOC |
| **SQN resync storm** | > 50 SQN resync requests for a single SUPI in 24 hours | Flag potential USIM cloning, alert SOC |
| **Credential scan** | Single NF queries auth credentials for > 1,000 distinct SUPIs in 10 minutes | Block NF, alert SOC |
| **Scope escalation** | NF requests scope not in its NRF profile | Log, alert SOC |
| **Off-hours admin access** | OAM API access outside maintenance windows | Alert SOC |
| **Certificate anomaly** | Client certificate with unexpected issuer or near-expiry CA | Alert PKI team |

### 9.4 Breach Notification Procedures

In the event of a confirmed security breach:

1. **Detection (T+0)** — SOC confirms the breach via SIEM alert correlation.
2. **Containment (T+15 min)** — Affected NF tokens are revoked at the NRF. Network policies are tightened. Compromised pods are isolated.
3. **Assessment (T+1 hour)** — Scope of data exposure is determined using audit logs. Affected SUPIs are identified.
4. **Credential rotation (T+4 hours)** — If K/OPc exposure is suspected, affected subscribers are flagged for USIM replacement. HN private keys are rotated if SUCI deconceal keys are compromised.
5. **Notification (T+24 hours)** — Regulatory bodies are notified per GDPR Article 33 (72-hour window). Affected subscribers are notified per Article 34 if high risk.
6. **Post-incident (T+1 week)** — Root cause analysis completed. Security controls updated. Lessons-learned document published.

---

## 10. Compliance Matrix

### 10.1 3GPP TS 33.501 Requirements Mapping

The following table maps key TS 33.501 requirements to their implementation in the UDM system:

| TS 33.501 Section | Requirement | Implementation | Status |
|---|---|---|---|
| §5.2.1 | Network access security between UE and network | UDM generates 5G HE AV for 5G-AKA | ✅ Implemented |
| §6.1.2 | 5G-AKA authentication procedure | Milenage-based AV generation in `nudm-ueau` | ✅ Implemented |
| §6.1.3 | EAP-AKA' authentication procedure | EAP-AKA' AV generation supported in `nudm-ueau` | ✅ Implemented |
| §6.12.2 | Subscription privacy (SUCI) | ECIES Profile A/B deconceal in `nudm-sdec` | ✅ Implemented |
| §6.12.3 | Home network public key provisioning | HSM-based key management, OTA distribution | ✅ Implemented |
| §6.12.4 | SUCI verification at home network | SUCI validation and deconceal with rate limiting | ✅ Implemented |
| §9.2 | SQN management | Per-subscriber SQN with resync, stored in YugabyteDB | ✅ Implemented |
| §9.4 | Concealment of SUPI | SUPI never in cleartext over air interface; redacted in logs | ✅ Implemented |
| §13.2 | TLS for SBI interfaces | TLS 1.3 mandatory, mTLS enforced | ✅ Implemented |
| §13.3 | OAuth2 for NF authorization | OAuth2 client credentials via NRF | ✅ Implemented |
| §13.3.1 | NF service access tokens | JWT-based tokens with per-service scopes | ✅ Implemented |
| §13.4 | NF mutual authentication | mTLS + NF identity verification | ✅ Implemented |
| §C.2.1 | ECIES Profile A (X25519) | Implemented in `nudm-sdec` with HSM | ✅ Implemented |
| §C.2.2 | ECIES Profile B (secp256r1) | Implemented in `nudm-sdec` with HSM | ✅ Implemented |
| §C.3 | Key derivation function | HMAC-SHA-256 based KDF per TS 33.220 | ✅ Implemented |

### 10.2 Additional Compliance Standards

| Standard | Scope | UDM Coverage |
|---|---|---|
| **GSMA NESAS** | Network Equipment Security Assurance | Secure development lifecycle, vulnerability management |
| **ISO 27001** | Information Security Management | Data classification, access control, audit logging |
| **GDPR** | EU Data Protection | Privacy by design, data minimization, right to erasure |
| **SOC 2 Type II** | Service Organization Controls | Audit logging, access controls, change management |
| **FIPS 140-2 Level 3** | Cryptographic Module Security | HSM compliance for key storage and operations |
| **NIST SP 800-53** | Security and Privacy Controls | Defense-in-depth, zero-trust, continuous monitoring |

---

## References

- 3GPP TS 33.501 — "Security architecture and procedures for 5G System" ([`docs/3gpp/33501-ia0.docx`](3gpp/33501-ia0.docx))
- 3GPP TS 33.102 — "3G Security; Security architecture"
- 3GPP TS 33.220 — "Generic Authentication Architecture (GAA)"
- 3GPP TS 29.503 — "Nudm Services" (SBI API specification)
- [architecture.md](architecture.md) — UDM High-Level Architecture
- [deployment.md](deployment.md) — UDM Deployment Architecture
- [sbi-api-design.md](sbi-api-design.md) — SBI API Design Guide
- OWASP API Security Top 10 (2023)
- NIST SP 800-57 — "Recommendation for Key Management"

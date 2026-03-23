# Kubernetes NetworkPolicy manifests for 5G UDM micro-segmentation.
#
# Based on: docs/security.md §7.2 (Pod-to-Pod Micro-Segmentation)
# Based on: docs/security.md §7.1 (Network Segmentation)
#
# Each UDM service has an allow-list ingress/egress policy that enforces
# least-privilege network access. Only authorized NF consumer pods can
# reach each service, and each service can only reach YugabyteDB and NRF.
#
# Three network planes (docs/security.md §7.1):
#   1. Signaling (SBI): NF-to-NF communication via Kubernetes ClusterIP + Istio mTLS
#   2. Data (DB):       UDM-to-YugabyteDB, dedicated subnet, no external routing
#   3. Management (OAM): Operations + monitoring, bastion host access only
#
# Apply all policies: kubectl apply -f deploy/networkpolicies/

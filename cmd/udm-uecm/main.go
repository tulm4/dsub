// Command udm-uecm is the entry point for the Nudm_UECM microservice.
//
// Based on: docs/service-decomposition.md §2.3 (udm-uecm)
// 3GPP: TS 29.503 Nudm_UECM — UE Context Management service
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-uecm: not yet fully wired — Phase 3 service skeleton")
	os.Exit(1)
}

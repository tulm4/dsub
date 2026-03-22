// Command udm-ueid is the entry point for the Nudm_UEID microservice.
//
// Based on: docs/service-decomposition.md §2.10 (udm-ueid)
// 3GPP: TS 29.503 Nudm_UEID — UE Identification (SUCI de-concealment)
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-ueid: not yet fully wired — Phase 4 service skeleton")
	os.Exit(1)
}

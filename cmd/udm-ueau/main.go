// Command udm-ueau is the entry point for the Nudm_UEAU microservice.
//
// Based on: docs/service-decomposition.md §2.1 (udm-ueau)
// 3GPP: TS 29.503 Nudm_UEAU — UE Authentication service
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-ueau: not yet fully wired — Phase 3 service skeleton")
	os.Exit(1)
}

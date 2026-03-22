// Command udm-ee is the entry point for the Nudm_EE microservice.
//
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
// 3GPP: TS 29.503 Nudm_EE — Event Exposure service
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-ee: not yet fully wired — Phase 5 service skeleton")
	os.Exit(1)
}

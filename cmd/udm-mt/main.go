// Command udm-mt is the entry point for the Nudm_MT microservice.
//
// Based on: docs/service-decomposition.md §2.6 (udm-mt)
// 3GPP: TS 29.503 Nudm_MT — Mobile Terminated service
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-mt: not yet fully wired — Phase 5 service skeleton")
	os.Exit(1)
}

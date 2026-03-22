// Command udm-pp is the entry point for the Nudm_PP microservice.
//
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
// 3GPP: TS 29.503 Nudm_PP — Parameter Provisioning service
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-pp: not yet fully wired — Phase 5 service skeleton")
	os.Exit(1)
}

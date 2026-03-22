// Command udm-sdm is the entry point for the Nudm_SDM microservice.
//
// Based on: docs/service-decomposition.md §2.2 (udm-sdm)
// 3GPP: TS 29.503 Nudm_SDM — Subscriber Data Management service
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "udm-sdm: not yet fully wired — Phase 3 service skeleton")
	os.Exit(1)
}

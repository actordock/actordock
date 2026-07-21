// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

// merge-eval-results merges policy_report_*.json into policy_compare.md.
package main

import (
	"fmt"
	"os"

	"github.com/actordock/actordock/e2e/eval"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <input-dir> <output.md>\n", os.Args[0])
		os.Exit(2)
	}
	path, err := eval.MergePolicyEvalDir(os.Args[1], os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "merge: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(path)
}

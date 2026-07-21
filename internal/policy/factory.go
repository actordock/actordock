// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

import "fmt"

// New returns a named policy.
func New(name string) (Policy, error) {
	switch name {
	case "random":
		return NewRandom(nil), nil
	case "fifo":
		return NewFIFO(), nil
	default:
		return nil, fmt.Errorf("unknown policy %q (want random|fifo)", name)
	}
}

// Copyright 2026 The Actordock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestWaitSuccess(t *testing.T) {
	mr := miniredis.RunT(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := Wait(ctx, mr.Addr(), nil); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestWaitCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := Wait(ctx, "127.0.0.1:1", nil)
	if err == nil {
		t.Fatal("expected error when redis is unreachable")
	}
}

func TestWaitEmptyAddr(t *testing.T) {
	ctx := context.Background()
	if err := Wait(ctx, "", nil); err == nil {
		t.Fatal("expected error for empty addr")
	}
}

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

package store

import (
	"context"
	"testing"
	"time"
)

func TestEnqueueDequeueTemplateBuild(t *testing.T) {
	t.Parallel()

	r := newTestRedis(t)
	ctx := context.Background()
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)
	job := TemplateBuildJob{
		TemplateID: "custom-app",
		BuildID:    "build-1",
		EnqueuedAt: now,
	}
	if err := r.EnqueueTemplateBuild(ctx, job); err != nil {
		t.Fatalf("EnqueueTemplateBuild: %v", err)
	}

	dequeued, err := r.DequeueTemplateBuild(ctx)
	if err != nil {
		t.Fatalf("DequeueTemplateBuild: %v", err)
	}
	if dequeued.TemplateID != job.TemplateID || dequeued.BuildID != job.BuildID {
		t.Fatalf("dequeued = %+v, want %+v", dequeued, job)
	}
}

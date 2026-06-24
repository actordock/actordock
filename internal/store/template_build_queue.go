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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const templateBuildQueueKey = "actordock:template-build-queue"

// TemplateBuildJob is a queued template build for the template-builder worker.
type TemplateBuildJob struct {
	TemplateID string    `json:"template_id"`
	BuildID    string    `json:"build_id"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

func validateTemplateBuildJob(job TemplateBuildJob) error {
	if strings.TrimSpace(job.TemplateID) == "" {
		return ErrCatalogTemplateIDEmpty
	}
	if strings.TrimSpace(job.BuildID) == "" {
		return ErrTemplateBuildIDEmpty
	}
	if job.EnqueuedAt.IsZero() {
		return fmt.Errorf("template build job enqueued_at is required")
	}
	return nil
}

func (r *Redis) EnqueueTemplateBuild(ctx context.Context, job TemplateBuildJob) error {
	if err := validateTemplateBuildJob(job); err != nil {
		return err
	}
	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal template build job: %w", err)
	}
	if err := r.client.LPush(ctx, templateBuildQueueKey, data).Err(); err != nil {
		return fmt.Errorf("redis enqueue template build %s/%s: %w", job.TemplateID, job.BuildID, err)
	}
	return nil
}

// DequeueTemplateBuild blocks until a job is available or the context is canceled.
func (r *Redis) DequeueTemplateBuild(ctx context.Context) (TemplateBuildJob, error) {
	for {
		result, err := r.client.BRPop(ctx, 0, templateBuildQueueKey).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return TemplateBuildJob{}, err
			}
			return TemplateBuildJob{}, fmt.Errorf("redis dequeue template build: %w", err)
		}
		if len(result) < 2 {
			continue
		}
		var job TemplateBuildJob
		if err := json.Unmarshal([]byte(result[1]), &job); err != nil {
			return TemplateBuildJob{}, fmt.Errorf("unmarshal template build job: %w", err)
		}
		if err := validateTemplateBuildJob(job); err != nil {
			return TemplateBuildJob{}, err
		}
		return job, nil
	}
}

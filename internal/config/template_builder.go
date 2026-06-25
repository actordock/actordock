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

package config

import "fmt"

type TemplateBuilder struct {
	Server
	RedisAddr                      string
	TemplateNamespace              string
	DefaultBaseTemplate            string
	BuildRegistry                  string
	BuildRegistryInsecure          bool
	LocalhostRegistryReplacement   string
	LocalRegistryHost              string
	BuildDataDir                   string
	TemplateBuildFilesDir          string
	BuildWorkDir                   string
	BuildPVCName                   string
	KanikoImage                    string
	KanikoJobActiveDeadlineSeconds int32
}

func TemplateBuilderFromEnv() (TemplateBuilder, error) {
	server, err := ServerFromEnv("template-builder", ":8080")
	if err != nil {
		return TemplateBuilder{}, err
	}
	buildDataDir := envOrDefault("ACTORDOCK_BUILD_DATA_DIR", "/var/lib/actordock/template-build")
	buildRegistry := envOrDefault("ACTORDOCK_BUILD_REGISTRY", "kind-registry:5000")
	buildRegistryInsecure := envOrDefault("ACTORDOCK_BUILD_REGISTRY_INSECURE", "true") == "true"
	localhostReplacement := envOrDefault("ACTORDOCK_LOCALHOST_REGISTRY_REPLACEMENT", "")
	if localhostReplacement == "" && buildRegistryInsecure {
		localhostReplacement = buildRegistry
	}
	localRegistryHost := envOrDefault("ACTORDOCK_LOCAL_REGISTRY_HOST", "")
	if localRegistryHost == "" && buildRegistryInsecure {
		localRegistryHost = "localhost:5001"
	}
	kanikoDeadline, err := envIntOrDefault("ACTORDOCK_KANIKO_JOB_ACTIVE_DEADLINE_SECONDS", 270)
	if err != nil {
		return TemplateBuilder{}, err
	}
	cfg := TemplateBuilder{
		Server:                         server,
		RedisAddr:                      envOrDefault("TEMPLATE_BUILDER_REDIS_ADDR", "redis.actordock.svc:6379"),
		TemplateNamespace:              envOrDefault("ACTORDOCK_TEMPLATE_NAMESPACE", "actordock"),
		DefaultBaseTemplate:            envOrDefault("ACTORDOCK_DEFAULT_BASE_TEMPLATE", "base"),
		BuildRegistry:                  buildRegistry,
		BuildRegistryInsecure:          buildRegistryInsecure,
		LocalhostRegistryReplacement:   localhostReplacement,
		LocalRegistryHost:              localRegistryHost,
		BuildDataDir:                   buildDataDir,
		TemplateBuildFilesDir:          envOrDefault("ACTORDOCK_TEMPLATE_BUILD_FILES_DIR", buildDataDir+"/files"),
		BuildWorkDir:                   envOrDefault("ACTORDOCK_BUILD_WORK_DIR", buildDataDir+"/work"),
		BuildPVCName:                   envOrDefault("ACTORDOCK_BUILD_PVC_NAME", "template-build-workdir"),
		KanikoImage:                    envOrDefault("ACTORDOCK_KANIKO_IMAGE", "gcr.io/kaniko-project/executor:v1.23.2"),
		KanikoJobActiveDeadlineSeconds: int32(kanikoDeadline),
	}
	if cfg.TemplateNamespace == "" {
		return TemplateBuilder{}, fmt.Errorf("ACTORDOCK_TEMPLATE_NAMESPACE is required")
	}
	if cfg.BuildRegistry == "" {
		return TemplateBuilder{}, fmt.Errorf("ACTORDOCK_BUILD_REGISTRY is required")
	}
	return cfg, nil
}

{{/*
Copyright 2026 The Actordock Authors. SPDX-License-Identifier: Apache-2.0
*/}}

{{- define "actordock-stack.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "actordock-stack.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "actordock-stack.namespace" -}}
{{- default .Release.Namespace .Values.namespace }}
{{- end }}

{{- define "actordock-stack.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "actordock-stack.labels" -}}
helm.sh/chart: {{ include "actordock-stack.chart" . }}
{{ include "actordock-stack.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "actordock-stack.selectorLabels" -}}
app.kubernetes.io/name: {{ include "actordock-stack.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "actordock-stack.componentLabels" -}}
{{ include "actordock-stack.labels" . }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "actordock-stack.componentSelectorLabels" -}}
{{ include "actordock-stack.selectorLabels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end }}

{{- define "actordock-stack.image" -}}
{{- $cfg := .image -}}
{{- $tag := default $.root.Chart.AppVersion $cfg.tag -}}
{{- printf "%s:%s" $cfg.repository $tag -}}
{{- end }}

{{- define "actordock-stack.redisAddr" -}}
{{- if .Values.actordock.redis.addr -}}
{{- .Values.actordock.redis.addr -}}
{{- else -}}
redis.{{ include "actordock-stack.namespace" . }}.svc:6379
{{- end -}}
{{- end }}

{{- define "actordock-stack.apiKeySecretName" -}}
{{- if .Values.secrets.existingSecret -}}
{{- .Values.secrets.existingSecret -}}
{{- else -}}
{{- printf "%s-api-key" (include "actordock-stack.fullname" .) -}}
{{- end -}}
{{- end }}

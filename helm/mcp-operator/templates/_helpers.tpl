{{/*
Expand the name of the chart.
*/}}
{{- define "mcp-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "mcp-operator.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "mcp-operator.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "mcp-operator.labels" -}}
helm.sh/chart: {{ include "mcp-operator.chart" . }}
{{ include "mcp-operator.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- with .Values.commonLabels }}
{{ toYaml . }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "mcp-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "mcp-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "mcp-operator.serviceAccountName" -}}
{{- if .Values.operator.serviceAccount.create }}
{{- default (include "mcp-operator.fullname" .) .Values.operator.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.operator.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Create the namespace to use
*/}}
{{- define "mcp-operator.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride }}
{{- end }}

{{/*
Create the name of the metrics service
*/}}
{{- define "mcp-operator.metricsServiceName" -}}
{{- printf "%s-metrics" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the webhook service
*/}}
{{- define "mcp-operator.webhookServiceName" -}}
{{- printf "%s-webhook" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the webhook configuration
*/}}
{{- define "mcp-operator.webhookConfigName" -}}
{{- printf "%s-webhook-config" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the leader election role
*/}}
{{- define "mcp-operator.leaderElectionRoleName" -}}
{{- printf "%s-leader-election" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the manager role
*/}}
{{- define "mcp-operator.managerRoleName" -}}
{{- printf "%s-manager" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the proxy role
*/}}
{{- define "mcp-operator.proxyRoleName" -}}
{{- printf "%s-proxy" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create the name of the metrics reader role
*/}}
{{- define "mcp-operator.metricsReaderRoleName" -}}
{{- printf "%s-metrics-reader" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Webhook certificate secret name
*/}}
{{- define "mcp-operator.webhookCertSecretName" -}}
{{- printf "%s-webhook-certs" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Webhook CA bundle
*/}}
{{- define "mcp-operator.webhookCABundle" -}}
{{- if .Values.webhook.certManager.enabled }}
{{- /* CA bundle will be injected by cert-manager */ -}}
{{- else }}
{{- /* Use provided CA bundle or empty for self-signed */ -}}
{{- .Values.webhook.caBundle | default "" }}
{{- end }}
{{- end }}

{{/*
Create webhook failure policy
*/}}
{{- define "mcp-operator.webhookFailurePolicy" -}}
{{- .Values.webhook.failurePolicy | default "Fail" }}
{{- end }}

{{/*
Create webhook admission review versions
*/}}
{{- define "mcp-operator.webhookAdmissionReviewVersions" -}}
{{- .Values.webhook.admissionReviewVersions | default (list "v1" "v1beta1") }}
{{- end }}

{{/*
Create webhook side effects
*/}}
{{- define "mcp-operator.webhookSideEffects" -}}
{{- .Values.webhook.sideEffects | default "None" }}
{{- end }}

{{/*
Create webhook timeout seconds
*/}}
{{- define "mcp-operator.webhookTimeoutSeconds" -}}
{{- .Values.webhook.timeoutSeconds | default 10 }}
{{- end }}

{{/*
Create PodDisruptionBudget name
*/}}
{{- define "mcp-operator.pdbName" -}}
{{- printf "%s-pdb" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Create NetworkPolicy name
*/}}
{{- define "mcp-operator.networkPolicyName" -}}
{{- printf "%s-network-policy" (include "mcp-operator.fullname" .) }}
{{- end }}

{{/*
Check if cert-manager is available
*/}}
{{- define "mcp-operator.certManagerAvailable" -}}
{{- $certManagerAvailable := false -}}
{{- if .Capabilities.APIVersions.Has "cert-manager.io/v1" -}}
{{- $certManagerAvailable = true -}}
{{- end -}}
{{- $certManagerAvailable -}}
{{- end }}

{{/*
Determine if webhooks should be enabled
*/}}
{{- define "mcp-operator.webhooksEnabled" -}}
{{- $webhooksEnabled := false -}}
{{- if .Values.webhook.enabled -}}
{{- if .Values.webhook.autoDetectCertManager -}}
{{- $certManagerAvailable := include "mcp-operator.certManagerAvailable" . -}}
{{- if eq $certManagerAvailable "true" -}}
{{- $webhooksEnabled = true -}}
{{- end -}}
{{- else -}}
{{- $webhooksEnabled = true -}}
{{- end -}}
{{- end -}}
{{- $webhooksEnabled -}}
{{- end }}

{{/*
Validate required values
*/}}
{{- define "mcp-operator.validateValues" -}}
{{- $webhooksEnabled := include "mcp-operator.webhooksEnabled" . -}}
{{- if and (eq $webhooksEnabled "true") (not .Values.webhook.certManager.enabled) (not .Values.webhook.caBundle) }}
{{- fail "webhook.caBundle is required when webhook is enabled and cert-manager is disabled" }}
{{- end }}
{{- if and .Values.metrics.serviceMonitor.enabled (not .Values.metrics.enabled) }}
{{- fail "metrics.enabled must be true when serviceMonitor is enabled" }}
{{- end }}
{{- end }}
# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

variable "instrument" {
  type        = bool
  description = "When true, the workload is defined through the Datadog module (APPLY). When false, the module is removed and no app exists (REMOVE) -- the clean end-state."
}

variable "subscription_id" {
  type     = string
  nullable = false
}

variable "resource_group_name" {
  type     = string
  nullable = false
}

variable "container_app_environment_id" {
  type     = string
  nullable = false
}

variable "name" {
  type        = string
  nullable    = false
  description = "Container App name. Must follow the one-e2e-<tool>-<platform>-<runid> hygiene convention."
}

variable "workload_profile_name" {
  type        = string
  default     = null
  description = "Workload profile to place the app in. Must be null for Consumption-Only environments."
}

variable "workload_image" {
  type        = string
  nullable    = false
  description = "Prebuilt prod self-monitoring workload image (sidecar flavor). Emits a log line and serves the HTTP trigger."
}

variable "datadog_api_key" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "datadog_site" {
  type    = string
  default = "datadoghq.com"
}

variable "datadog_service" {
  type        = string
  nullable    = false
  description = "DD_SERVICE for the instrumented app. Set to the unique app name so ingested telemetry is filterable by run id."
}

variable "datadog_env" {
  type     = string
  nullable = false
}

variable "datadog_version" {
  type     = string
  nullable = false
}

variable "run_id_tag" {
  type        = string
  nullable    = false
  description = "Unique run-id marker propagated to telemetry via DD_TAGS, e.g. one_e2e_run_id:deadbeef."
}

variable "created_ts" {
  type        = string
  nullable    = false
  description = "Unix timestamp set as the one_e2e_created freshness tag at creation, for the cross-repo sweeper."
}

variable "serverless_init_image" {
  type        = string
  nullable    = false
  description = "Pinned datadog/serverless-init sidecar image. Pinned so failures blame the module, not upstream."
}

# Registry credentials for pulling the workload image from a private ACR.
# Leave empty for a public image.
variable "registry_server" {
  type    = string
  default = ""
}

variable "registry_username" {
  type    = string
  default = ""
}

variable "registry_password" {
  type      = string
  default   = ""
  sensitive = true
}

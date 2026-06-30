# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

# E2E fixture: the workload defined through this repo's Datadog module (the tool under
# test). APPLY creates the instrumented Container App from nothing; toggling
# `var.instrument` off (REMOVE) destroys it, leaving a clean end-state (no app).

locals {
  use_registry = var.registry_server != ""

  registry = local.use_registry ? [{
    server               = var.registry_server
    username             = var.registry_username
    password_secret_name = "acr-pwd"
  }] : null

  registry_secret = local.use_registry ? [{
    name  = "acr-pwd"
    value = var.registry_password
  }] : null

  # one_e2e_created drives the cross-repo sweeper; it must be present at creation.
  freshness_tags = { one_e2e_created = var.created_ts }

  ingress = {
    external_enabled = true
    target_port      = 8080
    traffic_weight = [{
      percentage      = 100
      latest_revision = true
    }]
  }

  workload_container = {
    cpu    = 0.5
    memory = "1Gi"
    image  = var.workload_image
    name   = "main"
  }
}

module "instrumented" {
  count  = var.instrument ? 1 : 0
  source = "../.."

  name                         = var.name
  resource_group_name          = var.resource_group_name
  container_app_environment_id = var.container_app_environment_id
  revision_mode                = "Single"
  workload_profile_name        = var.workload_profile_name
  tags                         = local.freshness_tags

  datadog_api_key = var.datadog_api_key
  datadog_site    = var.datadog_site
  datadog_service = var.datadog_service
  datadog_env     = var.datadog_env
  datadog_version = var.datadog_version
  datadog_tags    = [var.run_id_tag]

  # Pin the sidecar artifact so a green/red result blames this module, not upstream.
  datadog_sidecar = {
    image = var.serverless_init_image
  }

  registry = local.registry
  secret   = local.registry_secret
  ingress  = local.ingress
  template = {
    min_replicas = 1
    max_replicas = 1
    container    = [local.workload_container]
  }
}

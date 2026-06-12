# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

variable "datadog_api_key" {
  type      = string
  sensitive = true
  nullable  = false
}

variable "name" {
  type     = string
  nullable = false
}

variable "resource_group_name" {
  type     = string
  nullable = false
}

variable "subscription_id" {
  type     = string
  nullable = false
}

variable "environment_name" {
  type     = string
  nullable = false
}

variable "image" {
  type     = string
  nullable = false
}


provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

module "example_container_app" {
  source          = "../../"
  datadog_api_key = var.datadog_api_key
  datadog_site    = "datadoghq.com"
  datadog_service = "terraform-test"
  datadog_env     = "dev"
  datadog_version = "1.0.0"

  name                         = var.name
  resource_group_name          = var.resource_group_name
  container_app_environment_id = "/subscriptions/${var.subscription_id}/resourceGroups/${var.resource_group_name}/providers/Microsoft.App/managedEnvironments/${var.environment_name}"

  revision_mode         = "Single"
  workload_profile_name = "Consumption"
  ingress = {
    external_enabled = true
    target_port      = 8080
    traffic_weight = [{
      percentage      = 100
      latest_revision = true
    }]
  }
  template = {
    container = [{
      cpu    = 0.5
      memory = "1Gi"
      image  = var.image
      name   = "main"
    }]
  }
}

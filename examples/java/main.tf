# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

terraform {
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = ">= 4.70.0"
    }
  }
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
  type     = string
  nullable = false
}

variable "datadog_env" {
  type     = string
  nullable = false
}

variable "datadog_version" {
  type    = string
  default = "1.0.0"
}


provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

data "azurerm_client_config" "current" {}

module "container_app" {
  source  = "DataDog/container-app-datadog/azurerm"
  version = "~> 1.1"

  name                         = var.name
  resource_group_name          = var.resource_group_name
  container_app_environment_id = "/subscriptions/${var.subscription_id}/resourceGroups/${var.resource_group_name}/providers/Microsoft.App/managedEnvironments/${var.environment_name}"

  datadog_api_key = var.datadog_api_key
  datadog_site    = var.datadog_site
  datadog_service = var.datadog_service
  datadog_env     = var.datadog_env
  datadog_version = var.datadog_version

  datadog_sidecar = {
    env = [
      { name = "DD_AZURE_SUBSCRIPTION_ID", value = data.azurerm_client_config.current.subscription_id },
      { name = "DD_AZURE_RESOURCE_GROUP", value = var.resource_group_name },
      { name = "DD_SOURCE", value = "java" },
    ]
  }

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
      env = [
        { name = "DD_LOGS_INJECTION", value = "true" },
        { name = "DD_APPSEC_ENABLED", value = "true" },
        # Enable the Datadog Java tracer (downloaded into the image) for automatic APM instrumentation.
        { name = "JAVA_TOOL_OPTIONS", value = "-javaagent:/app/agent.jar" },
      ]
    }]
  }

  tags = {
    service         = var.datadog_service
    dd_sls_mcp_tool = "true"
  }
}

output "app_url" {
  description = "The public URL of the Container App."
  value       = "https://${module.container_app.latest_revision_fqdn}"
}

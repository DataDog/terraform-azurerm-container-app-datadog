# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

output "app_fqdn" {
  description = "Ingress FQDN of the workload, used to trigger it over HTTP."
  value = var.instrument ? (
    try(module.instrumented[0].ingress[0].fqdn, null)
    ) : (
    try(azurerm_container_app.plain[0].ingress[0].fqdn, null)
  )
}

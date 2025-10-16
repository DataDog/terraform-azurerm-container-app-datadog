# Datadog Terraform module for Azure Container Apps

Use this Terraform module to install Datadog Serverless Monitoring for Azure Container Apps.

[This Terraform module](https://registry.terraform.io/modules/DataDog/container-app-datadog/azurerm/latest) wraps the [azurerm_container_app](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/container_app) resource and automatically configures your Container App for Datadog Serverless Monitoring by:

* creating the `azurerm_container_app` resource invocation
* adding the designated volumes, volume_mounts to the main container if the user enables logging
* adding the Datadog agent as a sidecar container to collect metrics, traces, and logs
* configuring environment variables for Datadog instrumentation


<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5.0 |
| <a name="requirement_azurerm"></a> [azurerm](#requirement\_azurerm) | >= 4.49.0 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [azurerm_container_app.this](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/resources/container_app) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_container_app_environment_id"></a> [container\_app\_environment\_id](#input\_container\_app\_environment\_id) | The ID of the Container App Environment to host this Container App. | `string` | n/a | yes |
| <a name="input_dapr"></a> [dapr](#input\_dapr) | n/a | <pre>object({<br/>    app_id       = string,<br/>    app_port     = optional(number),<br/>    app_protocol = optional(string)<br/>  })</pre> | `null` | no |
| <a name="input_identity"></a> [identity](#input\_identity) | n/a | <pre>object({<br/>    identity_ids = optional(set(string)),<br/>    type         = string<br/>  })</pre> | `null` | no |
| <a name="input_ingress"></a> [ingress](#input\_ingress) | n/a | <pre>object({<br/>    allow_insecure_connections = optional(bool),<br/>    client_certificate_mode    = optional(string),<br/>    exposed_port               = optional(number),<br/>    external_enabled           = optional(bool),<br/>    target_port                = number,<br/>    transport                  = optional(string),<br/>    cors = optional(object({<br/>      allow_credentials_enabled = optional(bool),<br/>      allowed_headers           = optional(list(string)),<br/>      allowed_methods           = optional(list(string)),<br/>      allowed_origins           = list(string),<br/>      exposed_headers           = optional(list(string)),<br/>      max_age_in_seconds        = optional(number)<br/>    })),<br/>    ip_security_restriction = optional(list(object({<br/>      action           = string,<br/>      description      = optional(string),<br/>      ip_address_range = string,<br/>      name             = string<br/>    }))),<br/>    traffic_weight = list(object({<br/>      label           = optional(string),<br/>      latest_revision = optional(bool),<br/>      percentage      = number,<br/>      revision_suffix = optional(string)<br/>    }))<br/>  })</pre> | `null` | no |
| <a name="input_max_inactive_revisions"></a> [max\_inactive\_revisions](#input\_max\_inactive\_revisions) | n/a | `number` | `null` | no |
| <a name="input_name"></a> [name](#input\_name) | The name for this Container App. | `string` | n/a | yes |
| <a name="input_registry"></a> [registry](#input\_registry) | n/a | <pre>list(object({<br/>    identity             = optional(string),<br/>    password_secret_name = optional(string),<br/>    server               = string,<br/>    username             = optional(string)<br/>  }))</pre> | `null` | no |
| <a name="input_resource_group_name"></a> [resource\_group\_name](#input\_resource\_group\_name) | n/a | `string` | n/a | yes |
| <a name="input_revision_mode"></a> [revision\_mode](#input\_revision\_mode) | n/a | `string` | n/a | yes |
| <a name="input_secret"></a> [secret](#input\_secret) | n/a | <pre>set(object({<br/>    identity            = optional(string),<br/>    key_vault_secret_id = optional(string),<br/>    name                = string,<br/>    value               = optional(string)<br/>  }))</pre> | `null` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | n/a | `map(string)` | `null` | no |
| <a name="input_template"></a> [template](#input\_template) | n/a | <pre>object({<br/>    max_replicas                     = optional(number),<br/>    min_replicas                     = optional(number),<br/>    termination_grace_period_seconds = optional(number),<br/>    azure_queue_scale_rule = optional(list(object({<br/>      name         = string,<br/>      queue_length = number,<br/>      queue_name   = string,<br/>      authentication = list(object({<br/>        secret_name       = string,<br/>        trigger_parameter = string<br/>      }))<br/>    }))),<br/>    container = list(object({<br/>      args    = optional(list(string)),<br/>      command = optional(list(string)),<br/>      cpu     = number,<br/>      image   = string,<br/>      memory  = string,<br/>      name    = string,<br/>      env = optional(list(object({<br/>        name        = string,<br/>        secret_name = optional(string),<br/>        value       = optional(string)<br/>      }))),<br/>      liveness_probe = optional(list(object({<br/>        failure_count_threshold = optional(number),<br/>        host                    = optional(string),<br/>        initial_delay           = optional(number),<br/>        interval_seconds        = optional(number),<br/>        port                    = number,<br/>        timeout                 = optional(number),<br/>        transport               = string,<br/>        header = optional(list(object({<br/>          name  = string,<br/>          value = string<br/>        })))<br/>      }))),<br/>      readiness_probe = optional(list(object({<br/>        failure_count_threshold = optional(number),<br/>        host                    = optional(string),<br/>        initial_delay           = optional(number),<br/>        interval_seconds        = optional(number),<br/>        port                    = number,<br/>        success_count_threshold = optional(number),<br/>        timeout                 = optional(number),<br/>        transport               = string,<br/>        header = optional(list(object({<br/>          name  = string,<br/>          value = string<br/>        })))<br/>      }))),<br/>      startup_probe = optional(list(object({<br/>        failure_count_threshold = optional(number),<br/>        host                    = optional(string),<br/>        initial_delay           = optional(number),<br/>        interval_seconds        = optional(number),<br/>        port                    = number,<br/>        timeout                 = optional(number),<br/>        transport               = string,<br/>        header = optional(list(object({<br/>          name  = string,<br/>          value = string<br/>        })))<br/>      }))),<br/>      volume_mounts = optional(list(object({<br/>        name     = string,<br/>        path     = string,<br/>        sub_path = optional(string)<br/>      })))<br/>    })),<br/>    custom_scale_rule = optional(list(object({<br/>      custom_rule_type = string,<br/>      metadata         = map(string),<br/>      name             = string,<br/>      authentication = optional(list(object({<br/>        secret_name       = string,<br/>        trigger_parameter = string<br/>      })))<br/>    }))),<br/>    http_scale_rule = optional(list(object({<br/>      concurrent_requests = string,<br/>      name                = string,<br/>      authentication = optional(list(object({<br/>        secret_name       = string,<br/>        trigger_parameter = optional(string)<br/>      })))<br/>    }))),<br/>    init_container = optional(list(object({<br/>      args    = optional(list(string)),<br/>      command = optional(list(string)),<br/>      cpu     = optional(number),<br/>      image   = string,<br/>      memory  = optional(string),<br/>      name    = string,<br/>      env = optional(list(object({<br/>        name        = string,<br/>        secret_name = optional(string),<br/>        value       = optional(string)<br/>      }))),<br/>      volume_mounts = optional(list(object({<br/>        name     = string,<br/>        path     = string,<br/>        sub_path = optional(string)<br/>      })))<br/>    }))),<br/>    tcp_scale_rule = optional(list(object({<br/>      concurrent_requests = string,<br/>      name                = string,<br/>      authentication = optional(list(object({<br/>        secret_name       = string,<br/>        trigger_parameter = optional(string)<br/>      })))<br/>    }))),<br/>    volume = optional(list(object({<br/>      mount_options = optional(string),<br/>      name          = string,<br/>      storage_name  = optional(string),<br/>      storage_type  = optional(string)<br/>    })))<br/>  })</pre> | n/a | yes |
| <a name="input_timeouts"></a> [timeouts](#input\_timeouts) | n/a | <pre>object({<br/>    create = optional(string),<br/>    delete = optional(string),<br/>    read   = optional(string),<br/>    update = optional(string)<br/>  })</pre> | `null` | no |
| <a name="input_workload_profile_name"></a> [workload\_profile\_name](#input\_workload\_profile\_name) | n/a | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_container_app_environment_id"></a> [container\_app\_environment\_id](#output\_container\_app\_environment\_id) | The ID of the Container App Environment to host this Container App. |
| <a name="output_custom_domain_verification_id"></a> [custom\_domain\_verification\_id](#output\_custom\_domain\_verification\_id) | The ID of the Custom Domain Verification for this Container App. |
| <a name="output_dapr"></a> [dapr](#output\_dapr) | n/a |
| <a name="output_id"></a> [id](#output\_id) | n/a |
| <a name="output_identity"></a> [identity](#output\_identity) | n/a |
| <a name="output_ingress"></a> [ingress](#output\_ingress) | n/a |
| <a name="output_latest_revision_fqdn"></a> [latest\_revision\_fqdn](#output\_latest\_revision\_fqdn) | The FQDN of the Latest Revision of the Container App. |
| <a name="output_latest_revision_name"></a> [latest\_revision\_name](#output\_latest\_revision\_name) | The name of the latest Container Revision. |
| <a name="output_location"></a> [location](#output\_location) | n/a |
| <a name="output_max_inactive_revisions"></a> [max\_inactive\_revisions](#output\_max\_inactive\_revisions) | n/a |
| <a name="output_name"></a> [name](#output\_name) | The name for this Container App. |
| <a name="output_outbound_ip_addresses"></a> [outbound\_ip\_addresses](#output\_outbound\_ip\_addresses) | n/a |
| <a name="output_registry"></a> [registry](#output\_registry) | n/a |
| <a name="output_resource_group_name"></a> [resource\_group\_name](#output\_resource\_group\_name) | n/a |
| <a name="output_revision_mode"></a> [revision\_mode](#output\_revision\_mode) | n/a |
| <a name="output_secret"></a> [secret](#output\_secret) | n/a |
| <a name="output_tags"></a> [tags](#output\_tags) | n/a |
| <a name="output_template"></a> [template](#output\_template) | n/a |
| <a name="output_timeouts"></a> [timeouts](#output\_timeouts) | n/a |
| <a name="output_workload_profile_name"></a> [workload\_profile\_name](#output\_workload\_profile\_name) | n/a |
<!-- END_TF_DOCS -->

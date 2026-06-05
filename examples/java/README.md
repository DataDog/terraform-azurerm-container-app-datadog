# Java Container App Example

This example demonstrates how to deploy a Java Spring Boot application to Azure Container Apps.

## What This Example Does

This example deploys a simple Java Spring Boot web server that:
- Listens on port 8080
- Exposes a single endpoint at `/` that returns "Hello Java World!"
- Logs each request in JSON format to stdout and the shared log volume using Logback

## Prerequisites

- Docker
- Terraform
- Azure CLI (authenticated)
- An Azure resource group
- An Azure Container App environment
- An Azure Container Registry

## Usage

### 1. Configure Variables

Create a `terraform.tfvars` file with the following contents:

```tfvars
name                = "my-java-app"
resource_group_name = "my-resource-group"
subscription_id     = "00000000-0000-0000-0000-000000000000"
environment_name    = "my-container-app-env"
image               = "myregistry.azurecr.io/java-example:latest"
```

### 2. Build and Push the Docker Image

```bash
docker buildx build --platform linux/amd64 -t "myregistry.azurecr.io/java-example:latest" ./src --push
```

### 3. Deploy with Terraform

```bash
terraform init
terraform apply -auto-approve
```

### 4. Access the Application

After deployment, Terraform will output the application URL (`app_url`). Visit the URL to see the "Hello Java World!" message.

## Cleanup

To destroy the resources:

```bash
terraform destroy -auto-approve
```

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_azurerm"></a> [azurerm](#requirement\_azurerm) | >= 4.70.0 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_container_app"></a> [container\_app](#module\_container\_app) | DataDog/container-app-datadog/azurerm | ~> 1.1 |

## Resources

| Name | Type |
|------|------|
| [azurerm_client_config.current](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs/data-sources/client_config) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_datadog_api_key"></a> [datadog\_api\_key](#input\_datadog\_api\_key) | n/a | `string` | n/a | yes |
| <a name="input_datadog_env"></a> [datadog\_env](#input\_datadog\_env) | n/a | `string` | n/a | yes |
| <a name="input_datadog_service"></a> [datadog\_service](#input\_datadog\_service) | n/a | `string` | n/a | yes |
| <a name="input_datadog_site"></a> [datadog\_site](#input\_datadog\_site) | n/a | `string` | `"datadoghq.com"` | no |
| <a name="input_datadog_version"></a> [datadog\_version](#input\_datadog\_version) | n/a | `string` | `"1.0.0"` | no |
| <a name="input_environment_name"></a> [environment\_name](#input\_environment\_name) | n/a | `string` | n/a | yes |
| <a name="input_image"></a> [image](#input\_image) | n/a | `string` | n/a | yes |
| <a name="input_name"></a> [name](#input\_name) | n/a | `string` | n/a | yes |
| <a name="input_resource_group_name"></a> [resource\_group\_name](#input\_resource\_group\_name) | n/a | `string` | n/a | yes |
| <a name="input_subscription_id"></a> [subscription\_id](#input\_subscription\_id) | n/a | `string` | n/a | yes |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_app_url"></a> [app\_url](#output\_app\_url) | The public URL of the Container App. |
<!-- END_TF_DOCS -->

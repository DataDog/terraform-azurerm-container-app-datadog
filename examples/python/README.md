# Python Container App with Datadog Example

This example demonstrates how to deploy a Python Flask application to Azure Container Apps with Datadog monitoring enabled.

## What This Example Does

This example deploys a simple Flask web server that:
- Listens on port 8080
- Exposes a single endpoint at `/` that returns "Hello Python World!"
- Integrates with Datadog for APM tracing, logging, and custom metrics
- Uses the Datadog Python tracer (`ddtrace`) for distributed tracing
- Sends custom metrics via DogStatsD
- Writes structured logs to a shared volume for log collection

## Prerequisites

- Docker
- Terraform
- Azure CLI (authenticated)
- An Azure resource group
- An Azure Container App environment
- An Azure Container Registry
- A Datadog API key

## Usage

### 1. Configure Variables

Create a `terraform.tfvars` file with the following contents:

```tfvars
datadog_api_key     = "your-datadog-api-key"
name                = "my-python-app"
resource_group_name = "my-resource-group"
subscription_id     = "00000000-0000-0000-0000-000000000000"
environment_name    = "my-container-app-env"
image               = "myregistry.azurecr.io/python-example:latest"
```

### 2. Build and Push the Docker Image

```bash
docker buildx build --platform linux/amd64 -t "myregistry.azurecr.io/python-example:latest" ./src --push
```

### 3. Deploy with Terraform

```bash
terraform init
terraform apply -auto-approve
```

### 4. Access the Application

After deployment, Terraform will output the application URL. Visit the URL to see the "Hello Python World!" message.

## Monitoring in Datadog

Once deployed, you can view the following in your Datadog account:
- **APM Traces**: Distributed traces for HTTP requests
- **Logs**: Application logs with trace correlation
- **Custom Metrics**: The example sends a sample distribution metric called `cloudrun-py-sample-metric`
- **Infrastructure**: Container metrics and resource usage

## Cleanup

To destroy the resources:

```bash
terraform destroy -auto-approve
```

<!-- BEGIN_TF_DOCS -->
## Requirements

No requirements.

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_example_container_app"></a> [example\_container\_app](#module\_example\_container\_app) | ../../ | n/a |

## Resources

No resources.

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_datadog_api_key"></a> [datadog\_api\_key](#input\_datadog\_api\_key) | n/a | `string` | n/a | yes |
| <a name="input_environment_name"></a> [environment\_name](#input\_environment\_name) | n/a | `string` | n/a | yes |
| <a name="input_image"></a> [image](#input\_image) | n/a | `string` | n/a | yes |
| <a name="input_name"></a> [name](#input\_name) | n/a | `string` | n/a | yes |
| <a name="input_resource_group_name"></a> [resource\_group\_name](#input\_resource\_group\_name) | n/a | `string` | n/a | yes |
| <a name="input_subscription_id"></a> [subscription\_id](#input\_subscription\_id) | n/a | `string` | n/a | yes |

## Outputs

No outputs.
<!-- END_TF_DOCS -->

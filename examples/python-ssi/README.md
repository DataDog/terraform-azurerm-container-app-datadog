# Python single-language instrumentation

This example deploys a tracer-free Flask application and uses the module's Azure Container Apps init container to inject the Datadog Python tracer. The image does not install or run `ddtrace`.

## Prerequisites

- Docker with `buildx`
- Terraform
- Azure CLI authenticated to your subscription
- An Azure resource group, Container Apps environment, and Container Registry
- A Datadog API key

## Deploy

Create `terraform.tfvars`:

```tfvars
datadog_api_key     = "your-datadog-api-key"
name                = "my-python-ssi-app"
resource_group_name = "my-resource-group"
subscription_id     = "00000000-0000-0000-0000-000000000000"
environment_name    = "my-container-app-env"
image               = "myregistry.azurecr.io/python-ssi:latest"
```

Build and push the tracer-free image, then deploy it:

```shell
docker buildx build --platform linux/amd64 -t myregistry.azurecr.io/python-ssi:latest ./src --push
terraform init
terraform apply -auto-approve
curl "$(terraform output -raw url)"
```

## Verify traces

Open Trace Explorer and filter for `service:my-python-ssi-app env:example`. The module injects the tracer through `datadog_apm_instrumentation`; you do not need to add `ddtrace` to the image.

## Cleanup

```shell
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

| Name | Description |
|------|-------------|
| <a name="output_url"></a> [url](#output\_url) | Public URL for the Container App. |
<!-- END_TF_DOCS -->

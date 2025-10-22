# Container Apps with Datadog monitoring examples

Examples for instrumenting Azure Container Apps with a Datadog agent sidecar container.

## Available Languages

- [Python](./python)
- [Node.js](./node/)
- [Go](./go/)
- [Java](./java/)
- [.NET](./dotnet/)
- [Ruby](./ruby/)
- [PHP](./php/)

## Quick Deploy

You can Quick Deploy all example apps using `./deploy.sh`, and tear everything down using `./destroy.sh`

## Manual Deploy

To manually deploy an app:
- Ensure docker, terraform, and the azure cli installed
- Ensure sure you have a resource group, container app env, and container registry set up (and are authenticated)
- In any example directory, add a `terraform.tfvars` file with the following contents:
    ```tfvars
    datadog_api_key     = ...
    name                = "my-app"
    resource_group_name = "my-resource-group"
    subscription_id     = "00000000-0000-0000-0000-000000000000"
    environment_name    = "my-container-app-env"
    image               = "registry.azurecr.io/image:latest"
    ```
- Run the following to build the image and deploy the app:
    ```shell
    docker buildx build --platform linux/amd64 -t "$image" ./src --push
    terraform init && terraform apply -auto-approve
    ```

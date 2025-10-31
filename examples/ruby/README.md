# Ruby Container App with Datadog Example

This example demonstrates how to deploy a Ruby Sinatra application to Azure Container Apps with Datadog monitoring enabled.

## What This Example Does

This example deploys a simple Sinatra web server that:
- Listens on port 8080
- Exposes a single endpoint at `/` that returns "Hello Ruby World!"
- Integrates with Datadog for APM tracing and logging
- Uses automatic instrumentation via `datadog/auto_instrument`
- Writes logs with trace correlation to a shared volume for log collection
- Automatically injects trace IDs and span IDs into log messages

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
name                = "my-ruby-app"
resource_group_name = "my-resource-group"
subscription_id     = "00000000-0000-0000-0000-000000000000"
environment_name    = "my-container-app-env"
image               = "myregistry.azurecr.io/ruby-example:latest"
```

### 2. Build and Push the Docker Image

```bash
docker buildx build --platform linux/amd64 -t "myregistry.azurecr.io/ruby-example:latest" ./src --push
```

### 3. Deploy with Terraform

```bash
terraform init
terraform apply -auto-approve
```

### 4. Access the Application

After deployment, Terraform will output the application URL. Visit the URL to see the "Hello Ruby World!" message.

## Monitoring in Datadog

Once deployed, you can view the following in your Datadog account:
- **APM Traces**: Distributed traces for HTTP requests with automatic Sinatra instrumentation
- **Logs**: Application logs with trace correlation using `Datadog::Tracing.log_correlation`
- **Infrastructure**: Container metrics and resource usage

## Cleanup

To destroy the resources:

```bash
terraform destroy -auto-approve
```

# PHP Container App with Datadog Example

This example demonstrates how to deploy a PHP Laravel application to Azure Container Apps with Datadog monitoring enabled.

## What This Example Does

This example deploys a simple Laravel web server that:
- Listens on port 8080
- Exposes a single endpoint at `/` that returns "Hello PHP World!"
- Integrates with Datadog for APM tracing and logging
- Uses Laravel's logging facade to write logs
- Writes logs to a shared volume for log collection
- Automatically traces HTTP requests

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
name                = "my-php-app"
resource_group_name = "my-resource-group"
subscription_id     = "00000000-0000-0000-0000-000000000000"
environment_name    = "my-container-app-env"
image               = "myregistry.azurecr.io/php-example:latest"
```

### 2. Build and Push the Docker Image

```bash
docker buildx build --platform linux/amd64 -t "myregistry.azurecr.io/php-example:latest" ./src --push
```

### 3. Deploy with Terraform

```bash
terraform init
terraform apply -auto-approve
```

### 4. Access the Application

After deployment, Terraform will output the application URL. Visit the URL to see the "Hello PHP World!" message.

## Monitoring in Datadog

Once deployed, you can view the following in your Datadog account:
- **APM Traces**: Distributed traces for HTTP requests with automatic Laravel instrumentation
- **Logs**: Application logs written to a shared volume
- **Infrastructure**: Container metrics and resource usage

## Cleanup

To destroy the resources:

```bash
terraform destroy -auto-approve
```

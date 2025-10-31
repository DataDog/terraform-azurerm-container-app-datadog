# .NET Container App with Datadog Example

This example demonstrates how to deploy a .NET (ASP.NET Core) application to Azure Container Apps with Datadog monitoring enabled.

## What This Example Does

This example deploys a simple ASP.NET Core web server that:
- Listens on port 8080
- Exposes a single endpoint at `/` that returns "Hello Dotnet World!"
- Integrates with Datadog for APM tracing and logging
- Uses Serilog for structured JSON logging
- Writes logs to both console and a shared volume for log collection
- Automatically correlates logs with traces

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
name                = "my-dotnet-app"
resource_group_name = "my-resource-group"
subscription_id     = "00000000-0000-0000-0000-000000000000"
environment_name    = "my-container-app-env"
image               = "myregistry.azurecr.io/dotnet-example:latest"
```

### 2. Build and Push the Docker Image

```bash
docker buildx build --platform linux/amd64 -t "myregistry.azurecr.io/dotnet-example:latest" ./src --push
```

### 3. Deploy with Terraform

```bash
terraform init
terraform apply -auto-approve
```

### 4. Access the Application

After deployment, Terraform will output the application URL. Visit the URL to see the "Hello Dotnet World!" message.

## Monitoring in Datadog

Once deployed, you can view the following in your Datadog account:
- **APM Traces**: Distributed traces for HTTP requests
- **Logs**: Structured JSON logs with trace correlation
- **Infrastructure**: Container metrics and resource usage

## Cleanup

To destroy the resources:

```bash
terraform destroy -auto-approve
```

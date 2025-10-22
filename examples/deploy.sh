#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

set -euo pipefail

if [[ -z "$DD_API_KEY" ]]; then
    echo "Error: DD_API_KEY environment variable is not set."
    exit 1
fi

if ! command -v terraform &>/dev/null; then
    echo "Error: terraform command not found. Please install Terraform."
    exit 1
fi

suffix=$(tr -cd '[:alnum:]' <<<"$USER")
sub_id=$(az account show --query id -o tsv)
export TF_IN_AUTOMATION=true

rg_name="tf-container-app-datadog-$suffix"
env_name="container-app-env-$suffix"
acr_name="acr$suffix"

echo "Ensuring resource group, container app environment, and ACR are created"
az group show -n "$rg_name" &>/dev/null || az group create --name "$rg_name" --location eastus2
az containerapp show -n "$env_name" -g "$rg_name" &>/dev/null || az containerapp env create --name "$env_name" --resource-group "$rg_name" --location eastus2 --logs-destination none
if ! az acr show --name "$acr_name" &>/dev/null; then
    az acr create --name "$acr_name" --resource-group "$rg_name" --location eastus2 --sku Standard
    az acr update --name "$acr_name" --anonymous-pull-enabled
    az acr login --name "$acr_name"
fi

for runtime in *; do
    if [[ ! -d "$runtime" ]]; then
        continue
    fi
    echo "Building and Deploying $runtime"
    cd "$runtime" || exit
    app_name="$runtime-containerapp-$suffix"
    image="${acr_name}.azurecr.io/${app_name}:latest"
    docker buildx build --platform linux/amd64 -t "$image" ./src --push

    echo "datadog_api_key = \"$DD_API_KEY\"
name                = \"$app_name\"
resource_group_name = \"$rg_name\"
subscription_id     = \"$sub_id\"
environment_name    = \"$env_name\"
image               = \"$image\"" >terraform.tfvars
    terraform init -upgrade || { echo "failed to init $runtime" && continue; }
    terraform apply -auto-approve -compact-warnings
    cd ..
done

echo "✅ All resources have been deployed successfully 🚀"
portal_url="https://portal.azure.com/#@$(az account show --query user.name -o tsv | cut -d@ -f2)/resource/subscriptions/$sub_id/resourceGroups/$rg_name/overview"
echo "Access your Azure Container Apps in the Azure Portal: $portal_url"

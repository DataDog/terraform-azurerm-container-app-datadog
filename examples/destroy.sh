#!/usr/bin/env bash
# Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
# This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2025 Datadog, Inc.

set -euo pipefail

suffix=$(tr -cd '[:alnum:]' <<<"$USER")

rg_name="tf-container-app-datadog-$suffix"
az group delete -n "$rg_name" --yes
for runtime in *; do
    if [[ ! -d "$runtime" ]]; then
        continue
    fi
    rm "$runtime/*.tfstate*"
done

echo "✅ All resources have been deleted"

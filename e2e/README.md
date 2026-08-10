# Container App E2E suite

## Lifecycle contract

The suite creates a run-scoped Azure Container App through this module, then:

1. verifies the pinned workload and sidecar images, shared logging volume, Datadog environment variables and API-key secret wiring;
2. invokes the workload and waits for matching spans and logs;
3. confirms the next Terraform plan has no diff;
4. removes the app and confirms it is gone; and
5. runs Terraform teardown plus an Azure fallback delete after a failure.

## Prerequisites

- Go 1.23 or later and Terraform 1.5 or later
- An authenticated Azure CLI with access to the target subscription, resource group, and Container App Environment
- `DD_API_KEY` and `DD_APP_KEY` for `ddserverless.datadoghq.com`

The suite defaults to the shared Serverless E2E Azure infrastructure: subscription `1dd25961-a5c7-45bf-a5ba-c1475d365cc7`, resource group `datadog-ci-e2e`, and Container App Environment `dd-ci-e2e-capp-env`. Set `AZURE_SUBSCRIPTION_ID`, `AZURE_RESOURCE_GROUP`, or `AZURE_CONTAINER_APP_ENV` to override them. The suite selects the `Consumption` workload profile when the environment exposes one; set `AZURE_CONTAINER_APP_WORKLOAD_PROFILE` to override it. `DD_SITE` defaults to `datadoghq.com`. The workload and serverless-init images are pinned by digest. `E2E_WORKLOAD_IMAGE` and `E2E_SERVERLESS_INIT_IMAGE` may override them for deliberate local testing; private images also require `E2E_ACR_SERVER`, `E2E_ACR_USERNAME`, and `E2E_ACR_PASSWORD`.

## Run locally

```bash
cd e2e && dd-auth --domain ddserverless.datadoghq.com -- go test -count=1 -v -timeout 20m ./...
```

The 20-minute timeout is a failure ceiling; a healthy run should finish in under 5 minutes. Container App creation and deletion each have a 10-minute provider timeout so a stuck Azure operation fails promptly enough to preserve cleanup time.

## CI

[The E2E workflow](../.github/workflows/e2e.yaml) runs for changes to Terraform, the suite, or the workflow in the canonical repository. Fork PRs do not receive cloud credentials. Azure authentication uses GitHub OIDC; Datadog authentication uses short-lived keys from `DataDog/dd-sts-action`.

CI requires `AZURE_CLIENT_ID_E2E`, `AZURE_TENANT_ID_E2E`, `AZURE_SUBSCRIPTION_ID_E2E`, `AZURE_RESOURCE_GROUP_E2E`, and `AZURE_CONTAINER_APP_ENV_E2E`. `DD_SITE_E2E` is optional. Missing authentication or configuration fails the job.

## Resource hygiene

Each app has a unique `one-e2e-tf-capp-<run-id>` name and exact `one_e2e_run_id` and `one_e2e_created` tags. Terraform removes the app during the lifecycle and runs a final teardown. The [cross-repository sweeper](https://github.com/DataDog/serverless-ci/tree/main/e2e/cleanup-functions/azure) removes stale resources after interrupted runs.

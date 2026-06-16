# Container App E2E Suite

End-to-end test for this module's instrumentation mechanism, conforming to the
[serverless instrumentation e2e spec](https://github.com/DataDog/serverless-ci/blob/main/e2e/spec.md).
It drives a real Azure Container App through the full lifecycle and asserts the
observable outcome -- instrumented config **and** flowing telemetry -- not just a
green `terraform apply`.

## What it does

One Go test (`go test ./...`) runs the lifecycle against the same workload, deployed
two ways under one name (see `fixture/`):

1. **Provision** the uninstrumented workload (a plain `azurerm_container_app`).
2. **APPLY** -- re-deploy wrapped by this module, then verify config: the
   `datadog-sidecar` (pinned `serverless-init` image), the shared `EmptyDir` volume +
   mounts, the required `DD_*` env vars, the `dd-api-key` secret, and unified-service
   tag identity (`service`/`env`/`version`).
3. **Trigger** the workload over HTTP so it emits a trace and a log line.
4. **Verify telemetry** -- poll the Datadog Spans and Logs APIs (15s × 20) for items
   carrying this run's identity (`service` + `env` + `version` + unique `one_e2e_run_id`
   tag). The filter asserts identity, not mere existence.
5. **APPLY again** -- assert idempotent (`terraform plan` shows no diff).
6. **REMOVE** -- drop the module wrapper (back to the plain resource) and assert the
   clean end-state: no sidecar, no shared volume, no `DD_*` env vars, no `dd-api-key`
   secret, no DD identity tags.
7. **Teardown** -- `terraform destroy` runs always, even on failure.

### Resource hygiene

Each run names its app `one-e2e-tf-capp-<runid>` (≤ 32 chars, the Container App limit)
and sets a `one_e2e_created:<unix-ts>` tag at creation. The cross-repo
[sweeper](https://github.com/DataDog/serverless-ci/blob/main/e2e/cleanup-functions/azure)
deletes stale `one-e2e-` resources, so teardown skips on cancelled CI never leak.

## Prerequisites

- **Go** ≥ 1.23, **Terraform** ≥ 1.5.0
- **Azure CLI** (`az`) with the `containerapp` extension. The `azurerm` provider and
  the verifier's `az containerapp show` both reuse your CLI login.
- A Container App **Environment** (shared, provisioned once per subscription -- see the
  [setup guide](https://github.com/DataDog/serverless-ci/blob/main/e2e/setup/azure/container-app/README.md)).
- A Datadog **API + APP key** for the org the telemetry lands in.

### Local auth

```bash
az login                              # leaves a CLI session both az and azurerm use
az extension add --name containerapp  # one-time
```

### Environment variables

| Variable | Required | Description |
| --- | --- | --- |
| `AZURE_SUBSCRIPTION_ID` | yes | Subscription holding the resource group + environment |
| `AZURE_RESOURCE_GROUP` | yes | Resource group for the ephemeral app |
| `AZURE_CONTAINER_APP_ENV` | yes | Container App Environment name |
| `DATADOG_API_KEY` | yes | Datadog API key (telemetry query) |
| `DATADOG_APP_KEY` | yes | Datadog application key (telemetry query) |
| `DD_SITE` | no | Datadog site (default `datadoghq.com`) |
| `E2E_WORKLOAD_IMAGE` | no | Prebuilt prod workload image (default: the `ddselfmonitoringprod` Node sidecar-flavor image) |
| `E2E_SERVERLESS_INIT_IMAGE` | no | Pinned `serverless-init` sidecar image (default `index.docker.io/datadog/serverless-init:3`) |
| `E2E_ACR_SERVER` / `E2E_ACR_USERNAME` / `E2E_ACR_PASSWORD` | if private | Registry creds so the Container App can pull the workload image |
| `SKIP_CONTAINER_APP_E2E_TESTS` | no | Set `true` to skip the suite |

The default workload image lives in the private `ddselfmonitoringprod` ACR; set the
`E2E_ACR_*` credentials (or point `E2E_WORKLOAD_IMAGE` at a public image) so the
Container App can pull it.

### Run

```bash
cd smoke_tests/e2e
go test -v -timeout 45m ./...
```

## CI

`.github/workflows/e2e.yaml` runs the suite on PRs behind a path filter (module
sources + `smoke_tests/e2e/`). It runs for real only when both the path filter matches
**and** this repo's OIDC federation is provisioned (`AZURE_CLIENT_ID_E2E` is set);
otherwise it sets `SKIP_CONTAINER_APP_E2E_TESTS=true` so the test self-skips and the
required check stays green -- on forks and before the infra is wired. Azure auth uses
GitHub → Azure OIDC federation (`azure/login`).

### Provisioning the CI infra (one-time, per repo)

Until these are set, the job stays green by self-skipping. To enable real runs, wire a
service principal with a federated credential for subject
`repo:DataDog/terraform-azurerm-container-app-datadog:*` and configure
([setup guide](https://github.com/DataDog/serverless-ci/blob/main/e2e/setup/azure/container-app/README.md)):

- **Repo variables:** `AZURE_CLIENT_ID_E2E`, `AZURE_TENANT_ID_E2E`,
  `AZURE_SUBSCRIPTION_ID_E2E`, `AZURE_RESOURCE_GROUP_E2E`, `AZURE_CONTAINER_APP_ENV_E2E`,
  `DD_SITE_E2E`, `E2E_WORKLOAD_IMAGE`, `E2E_SERVERLESS_INIT_IMAGE`, `E2E_ACR_SERVER`,
  `E2E_ACR_USERNAME`
- **Repo secrets:** `DATADOG_API_KEY_E2E`, `DATADOG_APP_KEY_E2E`, `E2E_ACR_PASSWORD`

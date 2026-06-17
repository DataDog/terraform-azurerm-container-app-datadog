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
| `E2E_SERVERLESS_INIT_IMAGE` | no | Pinned `serverless-init` sidecar image (default `index.docker.io/datadog/serverless-init:1.9.15`) |
| `E2E_ACR_SERVER` / `E2E_ACR_USERNAME` / `E2E_ACR_PASSWORD` | if private | Registry creds so the Container App can pull a private workload image |
| `SKIP_CONTAINER_APP_E2E_TESTS` | no | Set `true` to skip the suite |

The default `E2E_WORKLOAD_IMAGE` lives in the private `ddselfmonitoringprod` ACR. The
ephemeral Container App needs to pull it, so either:

- point `E2E_WORKLOAD_IMAGE` at an **anonymous-pull mirror** (what CI does --
  `dde2etfcapp.azurecr.io/self-monitoring-container-app-node-sidecar-prod:latest`, no
  credentials), or
- supply `E2E_ACR_*` registry credentials for the private source.

(Why a mirror rather than registry creds: the module's `secret` input can't carry a
registry password today -- a pre-existing typing bug makes the secret block's `for_each`
reject more than one secret -- so the suite avoids that path entirely.)

### Run

```bash
cd smoke_tests/e2e
go test -v -timeout 45m ./...
```

## CI

`.github/workflows/e2e.yaml` runs the suite on PRs behind a path filter (module
sources + `smoke_tests/e2e/`). When the path filter matches, the suite runs for real and
the Azure OIDC + dd-sts auth steps must succeed -- an auth or federation failure fails the
job loudly rather than self-skipping green. When no relevant files change it sets
`SKIP_CONTAINER_APP_E2E_TESTS=true` so the required check stays green. Azure auth uses
GitHub → Azure OIDC federation (`azure/login`).

### CI infra (provisioned)

The OIDC federation and config are already wired for this repo (see the IAM catalog in
`serverless-ci/e2e/iam-infra.md`). The job runs for real on PRs that touch the module or
suite, authenticating via a service principal federated to
`repo:DataDog/terraform-azurerm-container-app-datadog:*` with `Contributor` on the
`datadog-ci-e2e` resource group.

- **Repo variables:** `AZURE_CLIENT_ID_E2E`, `AZURE_TENANT_ID_E2E`,
  `AZURE_SUBSCRIPTION_ID_E2E`, `AZURE_RESOURCE_GROUP_E2E`, `AZURE_CONTAINER_APP_ENV_E2E`,
  `DD_SITE_E2E`, `E2E_WORKLOAD_IMAGE` (the anonymous-pull mirror), `E2E_SERVERLESS_INIT_IMAGE`
- **Datadog auth (dd-sts):** short-lived API + App keys minted at runtime via
  [`DataDog/dd-sts-action`](https://github.com/DataDog/dd-sts-action) under the
  `terraform-azurerm-container-app-datadog-e2e` policy -- no static Datadog keys in this repo.

`E2E_ACR_*` are unset because the workload image is pulled from an anonymous-pull mirror.

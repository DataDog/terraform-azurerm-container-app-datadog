// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

// Package e2e exercises the full lifecycle of the container-app-datadog Terraform module
// against a real Azure Container App and Datadog: APPLY the module and verify config,
// trigger it and verify telemetry flows, re-APPLY for idempotency, REMOVE and verify the
// app is gone, then always tear down.
//
// See README.md for the auth and environment prerequisites.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/require"

	e2eshared "github.com/DataDog/terraform-azurerm-container-app-datadog/e2e/shared"
)

// Pinned identity for the run. service is the unique app name (carries the run id), so
// ingested telemetry is uniquely attributable to this run.
const (
	fixtureEnv     = "e2e"
	fixtureVersion = "1.0.0"

	// One canonical runtime per platform (Node.js). Both images are pinned by digest
	// so a pass or failure reflects this module, not a mutable upstream tag.
	defaultWorkloadImage       = "dde2etfcapp.azurecr.io/self-monitoring-container-app-node-sidecar-prod@sha256:c55211a19ae3ef68fada20542825fbcd18f346e7f540622cfeb924ce732f5a4c"
	defaultServerlessInitImage = "index.docker.io/datadog/serverless-init@sha256:6fb7637628fdf31d536bc9c49fbe6304371df5e2ecdb15c1c2d5e2d66395c3a0"
	defaultSubscriptionID      = "1dd25961-a5c7-45bf-a5ba-c1475d365cc7"
	defaultResourceGroup       = "datadog-ci-e2e"
	defaultContainerAppEnv     = "dd-ci-e2e-capp-env"
)

// TestContainerAppE2E exercises the full instrumentation lifecycle against a real
// Azure Container App: APPLY the module (from nothing) -> verify config -> trigger ->
// verify telemetry -> re-apply (idempotent) -> remove -> verify the app is gone.
func TestContainerAppE2E(t *testing.T) {
	cfg := loadConfig(t)
	ctx := context.Background()
	environmentID, err := preflightAzure(ctx, cfg)
	require.NoError(t, err)

	runID := os.Getenv("E2E_RUN_ID")
	if runID == "" {
		runID = e2eshared.NewRunID()
	}
	name := e2eshared.ResourceName(sharedCfg, runID)
	createdTS := strconv.FormatInt(time.Now().Unix(), 10)
	runTag := fmt.Sprintf("%s:%s", e2eshared.DefaultRunIDTagKey, runID)
	exp := Expectations{
		Service:       name,
		Env:           fixtureEnv,
		Version:       fixtureVersion,
		RunID:         runID,
		RunTag:        runTag,
		CreatedTS:     createdTS,
		Site:          cfg.site,
		WorkloadImage: cfg.workloadImage,
		SidecarImage:  cfg.sidecarImage,
	}
	telID := telemetryIdentity{service: name, env: fixtureEnv, runTag: runTag}
	t.Logf("run id %s -> app %q", runID, name)

	opts := &terraform.Options{
		TerraformDir: "fixture",
		Vars: map[string]interface{}{
			"instrument":                   true,
			"subscription_id":              cfg.subscriptionID,
			"resource_group_name":          cfg.resourceGroup,
			"container_app_environment_id": environmentID,
			"name":                         name,
			"workload_image":               exp.WorkloadImage,
			"datadog_site":                 exp.Site,
			"datadog_service":              exp.Service,
			"datadog_env":                  exp.Env,
			"datadog_version":              exp.Version,
			"run_id":                       exp.RunID,
			"run_id_tag":                   exp.RunTag,
			"created_ts":                   exp.CreatedTS,
			"serverless_init_image":        exp.SidecarImage,
			"registry_server":              os.Getenv("E2E_ACR_SERVER"),
			"registry_username":            os.Getenv("E2E_ACR_USERNAME"),
		},
		// Secrets go through TF_VAR_* env vars, not -var, so Terratest never echoes
		// them into the (CI) logs.
		EnvVars: map[string]string{
			"TF_VAR_datadog_api_key":   cfg.apiKey,
			"TF_VAR_registry_password": os.Getenv("E2E_ACR_PASSWORD"),
		},
		RetryableTerraformErrors: retryableTerraformErrors,
		MaxRetries:               3,
		TimeBetweenRetries:       15 * time.Second,
		NoColor:                  true,
	}

	// Teardown always, even on failure or panic.
	defer func() {
		runPhase(t, "teardown", func() { terraform.Destroy(t, opts) })
	}()

	mustGetApp := func() containerApp {
		app, err := getContainerApp(ctx, cfg.subscriptionID, cfg.resourceGroup, name)
		require.NoError(t, err)

		return app
	}

	runPhase(t, "instrumentation deploy", func() {
		terraform.InitAndApply(t, opts)
	})
	runPhase(t, "config verification", func() {
		require.NoError(t, verifyInstrumented(mustGetApp(), exp))
	})

	var fqdn string
	runPhase(t, "invoke", func() {
		fqdn = terraform.Output(t, opts, "app_fqdn")
		require.NotEmpty(t, fqdn, "expected an ingress FQDN")
		triggerWorkload(t, fqdn)
	})
	runPhase(t, "telemetry wait", func() {
		checkTelemetryFlowing(t, ctx, fqdn, exp.Site, cfg.apiKey, cfg.appKey, telID)
	})
	runPhase(t, "idempotency check", func() {
		terraform.Apply(t, opts)
		require.Equal(t, 0, terraform.PlanExitCode(t, opts), "re-apply should be a no-op (no diff)")
	})
	runPhase(t, "destroy", func() {
		opts.Vars["instrument"] = false
		terraform.Apply(t, opts)
	})
	runPhase(t, "cleanup verification", func() {
		_, err := getContainerApp(ctx, cfg.subscriptionID, cfg.resourceGroup, name)
		require.Error(t, err, "container app should no longer exist after the module is removed")
		require.Contains(t, err.Error(), "ResourceNotFound", "expected an Azure not-found error, got: %v", err)
	})
}

// checkTelemetryFlowing asserts that both traces and logs carrying this run's identity
// reach Datadog. Spans and logs are polled concurrently on the same budget; the polls
// run off the test goroutine, so their results are asserted back on it.
func checkTelemetryFlowing(t *testing.T, ctx context.Context, fqdn, site, apiKey, appKey string, id telemetryIdentity) {
	t.Helper()
	client := e2eshared.NewTelemetryClient(site, apiKey, appKey)
	t.Logf("polling Datadog (%s) for telemetry matching: %s", site, id.query())

	// Drive continuous traffic for the duration of the poll: the serverless-init sidecar
	// tails the shared-volume log file from the END, so lines written before its tailer
	// attached sit behind the offset. Without fresh requests during the poll no new lines
	// are forwarded and logs never arrive (spans are unaffected -- the tracer ships over
	// HTTP). Stop once both polls return.
	tctx, stopTraffic := context.WithCancel(ctx)
	defer stopTraffic()
	go e2eshared.GenerateTraffic(tctx, "https://"+fqdn, 5*time.Second)

	type result struct {
		label string
		err   error
	}
	results := make(chan result, 2)
	go func() {
		results <- result{"spans", waitForTelemetry(ctx, t, "spans", client.SearchSpans, id)}
	}()
	go func() {
		results <- result{"logs", waitForTelemetry(ctx, t, "logs", client.SearchLogs, id)}
	}()
	var failures []string
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", r.label, r.err))
		}
	}
	require.Empty(t, failures, "telemetry did not flow: %v", failures)
}

// triggerWorkload issues HTTP GETs until the service answers (or the budget runs out),
// so the app emits a trace and a log line. Bounded retries; transient errors only.
func triggerWorkload(t *testing.T, fqdn string) {
	t.Helper()
	url := "https://" + fqdn
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}
	const attempts = 30
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				t.Logf("triggered %s -> %d", url, resp.StatusCode)

				return
			}
			t.Logf("[trigger] attempt %d/%d got %d", attempt, attempts, resp.StatusCode)
		} else {
			t.Logf("[trigger] attempt %d/%d error: %v", attempt, attempts, err)
		}
		if attempt < attempts {
			time.Sleep(10 * time.Second)
		}
	}
	require.Failf(t, "trigger failed", "workload at %s never answered", url)
}

func loadConfig(t *testing.T) testConfig {
	t.Helper()
	cfg := testConfig{
		subscriptionID: getEnv("AZURE_SUBSCRIPTION_ID", defaultSubscriptionID),
		resourceGroup:  getEnv("AZURE_RESOURCE_GROUP", defaultResourceGroup),
		environment:    getEnv("AZURE_CONTAINER_APP_ENV", defaultContainerAppEnv),
		apiKey:         os.Getenv("DD_API_KEY"),
		appKey:         os.Getenv("DD_APP_KEY"),
		site:           getEnv("DD_SITE", "datadoghq.com"),
		workloadImage:  getEnv("E2E_WORKLOAD_IMAGE", defaultWorkloadImage),
		sidecarImage:   getEnv("E2E_SERVERLESS_INIT_IMAGE", defaultServerlessInitImage),
	}

	var missing []string
	for _, required := range []struct {
		name  string
		value string
	}{
		{"DD_API_KEY", cfg.apiKey},
		{"DD_APP_KEY", cfg.appKey},
	} {
		if required.value == "" {
			missing = append(missing, required.name)
		}
	}
	require.Empty(t, missing, "missing required e2e configuration: %v", missing)

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

// Package e2e exercises the full lifecycle of the container-app-datadog Terraform module
// against a real Azure Container App and Datadog: provision an uninstrumented workload,
// APPLY the module and verify config, trigger it and verify telemetry flows, re-APPLY for
// idempotency, REMOVE and verify a clean end-state, then always tear down.
//
// See README.md for the auth and environment prerequisites. The suite is skipped unless
// SKIP_CONTAINER_APP_E2E_TESTS is unset/false.
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

	// One canonical runtime per platform (Node.js). Prebuilt prod self-monitoring
	// workload (sidecar flavor): emits a log line and serves the HTTP trigger.
	defaultWorkloadImage = "ddselfmonitoringprod.azurecr.io/self-monitoring-container-app-node-sidecar-prod:latest"

	// Pinned Datadog artifact so a pass/fail blames this module, not upstream.
	defaultServerlessInitImage = "index.docker.io/datadog/serverless-init:1.9.15"
)

// TestContainerAppE2E exercises the full instrumentation lifecycle against a real
// Azure Container App: provision uninstrumented -> instrument -> verify config ->
// trigger -> verify telemetry -> re-apply (idempotent) -> remove -> verify clean.
func TestContainerAppE2E(t *testing.T) {
	if os.Getenv("SKIP_CONTAINER_APP_E2E_TESTS") == "true" {
		t.Skip("SKIP_CONTAINER_APP_E2E_TESTS=true")
	}

	ctx := context.Background()
	subscriptionID := requireEnv(t, "AZURE_SUBSCRIPTION_ID")
	resourceGroup := requireEnv(t, "AZURE_RESOURCE_GROUP")
	envName := requireEnv(t, "AZURE_CONTAINER_APP_ENV")
	apiKey := requireEnv(t, "DATADOG_API_KEY")
	appKey := requireEnv(t, "DATADOG_APP_KEY")
	site := getEnv("DD_SITE", "datadoghq.com")

	environmentID := fmt.Sprintf(
		"/subscriptions/%s/resourceGroups/%s/providers/Microsoft.App/managedEnvironments/%s",
		subscriptionID, resourceGroup, envName,
	)

	runID := e2eshared.NewRunID()
	name := e2eshared.ResourceName(sharedCfg, runID)
	createdTS := strconv.FormatInt(time.Now().Unix(), 10)
	runTag := fmt.Sprintf("%s:%s", e2eshared.DefaultRunIDTagKey, runID)
	sidecarImage := getEnv("E2E_SERVERLESS_INIT_IMAGE", defaultServerlessInitImage)
	exp := Expectations{
		Service:      name,
		Env:          fixtureEnv,
		Version:      fixtureVersion,
		RunID:        runID,
		CreatedTS:    createdTS,
		SidecarImage: sidecarImage,
	}
	telID := telemetryIdentity{service: name, env: fixtureEnv, runTag: runTag}
	t.Logf("run id %s -> app %q", runID, name)

	opts := &terraform.Options{
		TerraformDir: "fixture",
		Vars: map[string]interface{}{
			"instrument":                   false,
			"subscription_id":              subscriptionID,
			"resource_group_name":          resourceGroup,
			"container_app_environment_id": environmentID,
			"name":                         name,
			"workload_image":               getEnv("E2E_WORKLOAD_IMAGE", defaultWorkloadImage),
			"datadog_site":                 site,
			"datadog_service":              exp.Service,
			"datadog_env":                  exp.Env,
			"datadog_version":              exp.Version,
			"run_id_tag":                   runTag,
			"created_ts":                   createdTS,
			"serverless_init_image":        sidecarImage,
			"registry_server":              os.Getenv("E2E_ACR_SERVER"),
			"registry_username":            os.Getenv("E2E_ACR_USERNAME"),
		},
		// Secrets go through TF_VAR_* env vars, not -var, so Terratest never echoes
		// them into the (CI) logs.
		EnvVars: map[string]string{
			"TF_VAR_datadog_api_key":   apiKey,
			"TF_VAR_registry_password": os.Getenv("E2E_ACR_PASSWORD"),
		},
		RetryableTerraformErrors: retryableTerraformErrors,
		MaxRetries:               3,
		TimeBetweenRetries:       15 * time.Second,
		NoColor:                  true,
	}

	// Teardown always, even on failure or panic.
	defer terraform.Destroy(t, opts)

	// The instrumented app (module-managed) and the uninstrumented app (a plain
	// resource) share one Azure name but are distinct Terraform addresses, so a flip
	// can't be a single in-place apply -- Terraform would create the replacement while
	// the old one still exists ("already exists"). Each transition therefore destroys
	// the current resource before creating the next.
	setMode := func(instrument bool) {
		opts.Vars["instrument"] = instrument
		terraform.Apply(t, opts)
	}

	mustGetApp := func() containerApp {
		app, err := getContainerApp(ctx, subscriptionID, resourceGroup, name)
		require.NoError(t, err)

		return app
	}

	// 1. Provision the uninstrumented workload; confirm it starts clean.
	terraform.InitAndApply(t, opts) // instrument=false
	require.NoError(t, verifyUninstrumented(mustGetApp()))

	// 2. APPLY: instrument (destroy baseline first to free the name), then verify config.
	terraform.Destroy(t, opts) // instrument=false -> remove the plain app
	setMode(true)
	require.NoError(t, verifyInstrumented(mustGetApp(), exp))

	// 3. Trigger the workload over HTTP.
	fqdn := terraform.Output(t, opts, "app_fqdn")
	require.NotEmpty(t, fqdn, "expected an ingress FQDN")
	triggerWorkload(t, fqdn)

	// 4. Verify telemetry (traces + logs) flows, filtered by this run's identity.
	checkTelemetryFlowing(t, ctx, fqdn, site, apiKey, appKey, telID)

	// 5. APPLY again: assert idempotent (no diff, no duplicate).
	terraform.Apply(t, opts)
	require.Equal(t, 0, terraform.PlanExitCode(t, opts), "re-apply should be a no-op (no diff)")

	// 6. REMOVE: drop the module wrapper (back to the plain app), then verify clean.
	terraform.Destroy(t, opts) // instrument=true -> remove the instrumented app
	setMode(false)
	require.NoError(t, verifyUninstrumented(mustGetApp()))
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
	stopTraffic := make(chan struct{})
	defer close(stopTraffic)
	go generateTraffic("https://"+fqdn, stopTraffic)

	type result struct {
		label string
		err   error
	}
	results := make(chan result, 2)
	go func() {
		results <- result{"spans", waitForTelemetry(ctx, "spans", client.SearchSpans, id)}
	}()
	go func() {
		results <- result{"logs", waitForTelemetry(ctx, "logs", client.SearchLogs, id)}
	}()
	for i := 0; i < 2; i++ {
		r := <-results
		require.NoErrorf(t, r.err, "telemetry: %s did not flow", r.label)
	}
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
		time.Sleep(10 * time.Second)
	}
	require.Failf(t, "trigger failed", "workload at %s never answered", url)
}

// generateTraffic drives the workload on a steady cadence until stop is closed, so the
// sidecar's file tailer (which reads from the end) always has fresh log lines to forward
// while the telemetry poll runs. Best-effort: errors are ignored, the telemetry
// assertions are what gate the test.
func generateTraffic(url string, stop <-chan struct{}) {
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}},
	}
	hit := func() {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
		}
	}

	hit() // don't wait a full interval to start producing logs
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			hit()
		}
	}
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	v := os.Getenv(key)
	require.NotEmptyf(t, v, "%s must be set", key)

	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/require"
)

// Pinned identity for the run. service is the unique app name (carries the run id),
// so ingested telemetry is uniquely attributable to this run.
const (
	fixtureEnv     = "e2e"
	fixtureVersion = "1.0.0"

	// One canonical runtime per platform (Node.js). Prebuilt prod self-monitoring
	// workload (sidecar flavor): emits a log line and serves the HTTP trigger.
	defaultWorkloadImage = "ddselfmonitoringprod.azurecr.io/self-monitoring-container-app-node-sidecar-prod:latest"

	// Pinned Datadog artifact so a pass/fail blames this module, not upstream.
	defaultServerlessInitImage = "index.docker.io/datadog/serverless-init:3"
)

// TestContainerAppE2E exercises the full instrumentation lifecycle against a real
// Azure Container App: provision uninstrumented -> instrument -> verify config ->
// trigger -> verify telemetry -> re-apply (idempotent) -> remove -> verify clean.
func TestContainerAppE2E(t *testing.T) {
	if os.Getenv("SKIP_CONTAINER_APP_E2E_TESTS") == "true" {
		t.Skip("SKIP_CONTAINER_APP_E2E_TESTS=true")
	}

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

	runID := newRunID()
	name := appName(runID)
	createdTS := createdTimestamp()
	id := identity{
		service:      name,
		env:          fixtureEnv,
		version:      fixtureVersion,
		runTag:       runIDTag(runID),
		createdTS:    createdTS,
		sidecarImage: getEnv("E2E_SERVERLESS_INIT_IMAGE", defaultServerlessInitImage),
	}
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
			"datadog_api_key":              apiKey,
			"datadog_site":                 site,
			"datadog_service":              id.service,
			"datadog_env":                  id.env,
			"datadog_version":              id.version,
			"run_id_tag":                   id.runTag,
			"created_ts":                   createdTS,
			"serverless_init_image":        id.sidecarImage,
			"registry_server":              os.Getenv("E2E_ACR_SERVER"),
			"registry_username":            os.Getenv("E2E_ACR_USERNAME"),
			"registry_password":            os.Getenv("E2E_ACR_PASSWORD"),
		},
		RetryableTerraformErrors: retryableTerraformErrors,
		MaxRetries:               3,
		TimeBetweenRetries:       15 * time.Second,
		NoColor:                  true,
	}

	// Teardown always, even on failure or panic.
	defer terraform.Destroy(t, opts)

	// 1. Provision the uninstrumented workload.
	terraform.InitAndApply(t, opts)

	// 2. APPLY: instrument, then verify config.
	opts.Vars["instrument"] = true
	terraform.Apply(t, opts)
	verifyInstrumented(t, getContainerApp(t, subscriptionID, resourceGroup, name), id)

	// 3. Trigger the workload over HTTP.
	fqdn := terraform.Output(t, opts, "app_fqdn")
	require.NotEmpty(t, fqdn, "expected an ingress FQDN")
	triggerWorkload(t, fqdn)

	// 4. Verify telemetry (traces + logs) flows, filtered by this run's identity.
	checkTelemetryFlowing(t, apiKey, appKey, site, id)

	// 5. APPLY again: assert idempotent (no diff, no duplicate).
	terraform.Apply(t, opts)
	require.Equal(t, 0, terraform.PlanExitCode(t, opts), "re-apply should be a no-op (no diff)")

	// 6. REMOVE: drop the module wrapper, then verify the clean end-state.
	opts.Vars["instrument"] = false
	terraform.Apply(t, opts)
	verifyUninstrumented(t, getContainerApp(t, subscriptionID, resourceGroup, name))
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

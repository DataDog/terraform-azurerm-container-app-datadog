// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

// Package e2e exercises the full lifecycle of the container-app-datadog Terraform module
// against a real Azure Container App and Datadog: APPLY the module and verify config,
// trigger it and verify telemetry flows, assert the next plan is empty, REMOVE and verify
// the app is gone, then tear down after failures.
//
// See README.md for the auth and environment prerequisites.
package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	ssiRegistry                = "dde2etfcapp.azurecr.io"
	defaultSubscriptionID      = "1dd25961-a5c7-45bf-a5ba-c1475d365cc7"
	defaultResourceGroup       = "datadog-ci-e2e"
	defaultContainerAppEnv     = "dd-ci-e2e-capp-env"
)

type e2eScenario struct {
	name            string
	runIDSuffix     string
	workloadImage   string
	instrumentation *instrumentationExpectations
}

// TestContainerAppE2E runs the existing tracer-in-image baseline and all six SSI
// runtimes concurrently. Each subtest has an isolated fixture, state, resource, and
// telemetry identity.
func TestContainerAppE2E(t *testing.T) {
	cfg := loadConfig(t)
	ctx := context.Background()
	environment, err := preflightAzure(ctx, cfg)
	require.NoError(t, err)
	if cfg.workloadProfile == "" {
		for _, profile := range environment.WorkloadProfiles {
			if profile.Name == "Consumption" {
				cfg.workloadProfile = profile.Name
				break
			}
		}
	}

	baseRunID := os.Getenv("E2E_RUN_ID")
	if baseRunID == "" {
		baseRunID = e2eshared.NewRunID()
	}
	if len(baseRunID) > 8 {
		baseRunID = baseRunID[:8]
	}

	for _, scenario := range e2eScenarios(cfg) {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			runContainerAppScenario(t, cfg, environment.ID, baseRunID, scenario)
		})
	}
}

func runContainerAppScenario(
	t *testing.T,
	cfg testConfig,
	environmentID string,
	baseRunID string,
	scenario e2eScenario,
) {
	t.Helper()
	ctx := context.Background()
	runID := baseRunID + "-" + scenario.runIDSuffix
	name := e2eshared.ResourceName(sharedCfg, runID)
	createdTS := strconv.FormatInt(time.Now().Unix(), 10)
	runTag := fmt.Sprintf("%s:%s", e2eshared.DefaultRunIDTagKey, runID)
	exp := expectations{
		Service:         name,
		Env:             fixtureEnv,
		Version:         fixtureVersion,
		RunID:           runID,
		RunTag:          runTag,
		Site:            cfg.site,
		WorkloadImage:   scenario.workloadImage,
		SidecarImage:    cfg.sidecarImage,
		Instrumentation: scenario.instrumentation,
	}
	telID := e2eshared.IdentityFor(sharedCfg, name, fixtureEnv, "", runID)
	t.Logf("run id %s -> app %q", runID, name)

	opts := &terraform.Options{
		TerraformDir: copyTerraformFixture(t),
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
			"created_ts":                   createdTS,
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
	if cfg.workloadProfile != "" {
		opts.Vars["workload_profile_name"] = cfg.workloadProfile
	}
	if scenario.instrumentation != nil {
		opts.Vars["datadog_apm_instrumentation"] = map[string]interface{}{
			"language": scenario.instrumentation.Language,
		}
	}

	cleanupComplete := false
	defer func() {
		if cleanupComplete {
			return
		}
		runPhase(t, "teardown", func() {
			if _, err := terraform.DestroyE(t, opts); err != nil {
				t.Logf("Terraform teardown failed: %v", err)
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := deleteContainerApp(cleanupCtx, cfg.subscriptionID, cfg.resourceGroup, name); err != nil {
				t.Errorf("Azure fallback cleanup failed for %q: %v", name, err)
			}
		})
	}()

	mustGetApp := func() containerApp {
		app, err := getContainerApp(ctx, cfg.subscriptionID, cfg.resourceGroup, name)
		require.NoError(t, err)

		return app
	}

	runPhase(t, "instrumentation deploy", func() {
		terraform.InitAndApply(t, opts)
	})
	var fqdn string
	runPhase(t, "config verification", func() {
		app := mustGetApp()
		require.NoError(t, verifyInstrumented(app, exp))
		fqdn = app.Properties.Configuration.Ingress.FQDN
		require.NotEmpty(t, fqdn, "expected an ingress FQDN")
	})

	runPhase(t, "invoke", func() {
		triggerWorkload(t, fqdn)
	})
	runPhase(t, "telemetry wait", func() {
		checkTelemetryFlowing(t, ctx, fqdn, exp.Site, cfg.apiKey, cfg.appKey, telID)
	})
	runPhase(t, "idempotency check", func() {
		require.Equal(t, 0, terraform.PlanExitCode(t, opts), "next plan should have no diff")
	})
	runPhase(t, "destroy", func() {
		opts.Vars["instrument"] = false
		terraform.Apply(t, opts)
	})
	runPhase(t, "cleanup verification", func() {
		_, err := getContainerApp(ctx, cfg.subscriptionID, cfg.resourceGroup, name)
		require.Error(t, err, "container app should no longer exist after the module is removed")
		require.True(t, isContainerAppNotFound(err), "expected an Azure not-found error, got: %v", err)
	})
	cleanupComplete = true
}

func e2eScenarios(cfg testConfig) []e2eScenario {
	return []e2eScenario{
		{
			name:          "baseline",
			runIDSuffix:   "base",
			workloadImage: cfg.workloadImage,
		},
		ssiScenario("java", "java", "E2E_SSI_JAVA_IMAGE", map[string]string{
			"JAVA_TOOL_OPTIONS": "-javaagent:/datadog-lib/dd-java-agent.jar -XX:+IgnoreUnrecognizedVMOptions",
		}),
		ssiScenario("node", "js", "E2E_SSI_NODE_IMAGE", map[string]string{
			"NODE_OPTIONS": "--require /datadog-lib/node_modules/dd-trace/init.js",
		}),
		ssiScenario("dotnet", "dotnet", "E2E_SSI_DOTNET_IMAGE", map[string]string{
			"CORECLR_ENABLE_PROFILING": "1",
			"CORECLR_PROFILER":         "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}",
			"CORECLR_PROFILER_PATH":    "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so",
			"DD_DOTNET_TRACER_HOME":    tracerMountPath,
			"LD_PRELOAD":               "/datadog-lib/continuousprofiler/Datadog.Linux.ApiWrapper.x64.so",
		}),
		ssiScenario("python", "python", "E2E_SSI_PYTHON_IMAGE", map[string]string{
			"PYTHONPATH": tracerMountPath,
		}),
		ssiScenario("ruby", "ruby", "E2E_SSI_RUBY_IMAGE", map[string]string{
			"RUBYOPT": "-r/datadog-lib/auto_inject",
		}),
		ssiScenario("php", "php", "E2E_SSI_PHP_IMAGE", map[string]string{
			"PHP_INI_SCAN_DIR":       ":/datadog-lib/linux-gnu/loader",
			"DD_LOADER_PACKAGE_PATH": tracerMountPath,
		}),
	}
}

func ssiScenario(name, language, imageEnv string, loaderEnv map[string]string) e2eScenario {
	return e2eScenario{
		name:          name,
		runIDSuffix:   name,
		workloadImage: getEnv(imageEnv, fmt.Sprintf("%s/%s-ssi:latest", ssiRegistry, name)),
		instrumentation: &instrumentationExpectations{
			Language:    language,
			TracerImage: fmt.Sprintf("datadoghq.azurecr.io/dd-lib-%s-init:latest", language),
			LoaderEnv:   loaderEnv,
		},
	}
}

// copyTerraformFixture gives each parallel scenario its own Terraform working directory
// and state while preserving the fixture's relative module source path.
func copyTerraformFixture(t *testing.T) string {
	t.Helper()
	moduleRoot, err := filepath.Abs("..")
	require.NoError(t, err)
	tempModuleRoot := filepath.Join(t.TempDir(), "module")
	tempFixture := filepath.Join(tempModuleRoot, "e2e", "fixture")
	require.NoError(t, os.MkdirAll(tempFixture, 0o755))

	copyFiles := func(pattern, destination string) {
		files, err := filepath.Glob(pattern)
		require.NoError(t, err)
		require.NotEmpty(t, files, "no Terraform files matched %s", pattern)
		for _, source := range files {
			contents, err := os.ReadFile(source)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(destination, filepath.Base(source)), contents, 0o644))
		}
	}
	copyFiles(filepath.Join(moduleRoot, "*.tf"), tempModuleRoot)
	copyFiles(filepath.Join(moduleRoot, "e2e", "fixture", "*.tf"), tempFixture)

	return tempFixture
}

// checkTelemetryFlowing asserts that both traces and logs carrying this run's identity
// reach Datadog. Spans and logs are polled concurrently on the same budget; the polls
// run off the test goroutine, so their results are asserted back on it.
func checkTelemetryFlowing(t *testing.T, ctx context.Context, fqdn, site, apiKey, appKey string, id e2eshared.Identity) {
	t.Helper()
	client := e2eshared.NewTelemetryClient(site, apiKey, appKey)
	t.Logf("polling Datadog (%s) for telemetry from service %q", site, id.Service)

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
		_, err := client.WaitForMatching(ctx, "spans", client.SearchSpans, e2eshared.SpanQuery(id), id)
		results <- result{"spans", err}
	}()
	go func() {
		_, err := client.WaitForMatching(ctx, "logs", client.SearchLogs, e2eshared.LogQuery(id), id)
		results <- result{"logs", err}
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
		subscriptionID:  getEnv("AZURE_SUBSCRIPTION_ID", defaultSubscriptionID),
		resourceGroup:   getEnv("AZURE_RESOURCE_GROUP", defaultResourceGroup),
		environment:     getEnv("AZURE_CONTAINER_APP_ENV", defaultContainerAppEnv),
		workloadProfile: os.Getenv("AZURE_CONTAINER_APP_WORKLOAD_PROFILE"),
		apiKey:          os.Getenv("DD_API_KEY"),
		appKey:          os.Getenv("DD_APP_KEY"),
		site:            getEnv("DD_SITE", "datadoghq.com"),
		workloadImage:   getEnv("E2E_WORKLOAD_IMAGE", defaultWorkloadImage),
		sidecarImage:    getEnv("E2E_SERVERLESS_INIT_IMAGE", defaultServerlessInitImage),
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

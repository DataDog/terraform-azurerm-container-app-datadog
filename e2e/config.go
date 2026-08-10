// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

// Repo-local config + Container App config/telemetry verification for the e2e suite. The
// generic, cross-cloud helpers (exec/retry, telemetry, naming, verification primitives)
// come from the shared e2eshared package; what lives here is everything specific to this
// module: the Azure retry patterns, the Container App "config present / clean"
// assertions, and the telemetry identity match (service + env + run-id, version
// deliberately omitted -- see telemetry.go).
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	e2eshared "github.com/DataDog/terraform-azurerm-container-app-datadog/e2e/shared"
)

// Azure Container App names are capped at 32 chars (the tightest budget across
// platforms); one-e2e-tf-capp-<8 hex> == 24 chars fits.
const containerAppNameMaxLen = 32

// sharedCfg parameterizes the shared helpers for this module: the Azure CLI, the Azure
// transient-error patterns safe to retry, and the tool/platform naming.
var sharedCfg = e2eshared.Config{
	Tool:       "tf",
	Platform:   "capp",
	Command:    "az",
	NameMaxLen: containerAppNameMaxLen,
	// We retry the cloud, not the assertions: never retry past a real failure.
	RetryPatterns: []string{
		"GatewayTimeout",
		"TooManyRequests",
		"Conflict", // a stale revision/resource still being deleted
		"OperationNotAllowed",
		"ETIMEDOUT",
		"ECONNRESET",
		"temporarily unavailable",
		"ServiceUnavailable",
		"InternalServerError",
	},
}

// retryableTerraformErrors are surfaced to Terratest so it retries apply/destroy on the
// same transient conditions.
var retryableTerraformErrors = map[string]string{
	".*Conflict.*":                "Resource conflict, likely a delete still in flight; retrying.",
	".*TooManyRequests.*":         "Azure throttling; retrying.",
	".*GatewayTimeout.*":          "Azure gateway timeout; retrying.",
	".*ServiceUnavailable.*":      "Azure service unavailable; retrying.",
	".*InternalServerError.*":     "Azure internal error; retrying.",
	".*temporarily unavailable.*": "Transient unavailability; retrying.",
}

// Subsets of Azure CLI JSON used by preflight and conformance checks.
type (
	containerAppEnvironment struct {
		ID               string `json:"id"`
		WorkloadProfiles []struct {
			Name string `json:"name"`
		} `json:"workloadProfiles"`
	}
	caEnvVar struct {
		Name      string `json:"name"`
		Value     string `json:"value"`
		SecretRef string `json:"secretRef"`
	}
	caVolumeMount struct {
		VolumeName string `json:"volumeName"`
		MountPath  string `json:"mountPath"`
	}
	caContainer struct {
		Name         string          `json:"name"`
		Image        string          `json:"image"`
		Env          []caEnvVar      `json:"env"`
		VolumeMounts []caVolumeMount `json:"volumeMounts"`
	}
	caVolume struct {
		Name        string `json:"name"`
		StorageType string `json:"storageType"`
	}
	containerApp struct {
		Properties struct {
			Template struct {
				Containers []caContainer `json:"containers"`
				Volumes    []caVolume    `json:"volumes"`
			} `json:"template"`
			Configuration struct {
				Ingress struct {
					FQDN string `json:"fqdn"`
				} `json:"ingress"`
				Secrets []struct {
					Name string `json:"name"`
				} `json:"secrets"`
			} `json:"configuration"`
		} `json:"properties"`
		Tags map[string]string `json:"tags"`
	}
)

const (
	sidecarName       = "datadog-sidecar"
	workloadName      = "main"
	sharedVolumeName  = "shared-volume"
	apiKeySecretName  = "dd-api-key"
	moduleMarkerTag   = "dd_sls_terraform_module"
	serverlessInitRef = "serverless-init"
	logPath           = "/shared-volume/logs/*.log"
)

// Expectations pins what an instrumented workload must look like, so a mismatch blames
// the module wiring rather than upstream drift.
type testConfig struct {
	subscriptionID  string
	resourceGroup   string
	environment     string
	workloadProfile string
	apiKey          string
	appKey          string
	site            string
	workloadImage   string
	sidecarImage    string
}

type Expectations struct {
	Service       string
	Env           string
	Version       string
	RunID         string
	RunTag        string
	CreatedTS     string
	Site          string
	WorkloadImage string
	SidecarImage  string
}

func preflightAzure(ctx context.Context, cfg testConfig) (containerAppEnvironment, error) {
	account, err := e2eshared.Run(ctx, sharedCfg,
		"account", "show",
		"--subscription", cfg.subscriptionID,
		"--query", "id",
		"--output", "tsv",
		"--only-show-errors",
	)
	if err != nil {
		return containerAppEnvironment{}, fmt.Errorf("Azure credential preflight: %w", err)
	}
	if account.Stdout != cfg.subscriptionID {
		return containerAppEnvironment{}, fmt.Errorf("Azure credential preflight returned subscription %q, want %q", account.Stdout, cfg.subscriptionID)
	}

	result, err := e2eshared.Run(ctx, sharedCfg,
		"containerapp", "env", "show",
		"--subscription", cfg.subscriptionID,
		"--resource-group", cfg.resourceGroup,
		"--name", cfg.environment,
		"--query", "{id:id,workloadProfiles:properties.workloadProfiles[].{name:name}}",
		"--output", "json",
		"--only-show-errors",
	)
	if err != nil {
		return containerAppEnvironment{}, fmt.Errorf("Container App Environment preflight: %w", err)
	}

	var environment containerAppEnvironment
	if err := json.Unmarshal([]byte(result.Stdout), &environment); err != nil {
		return containerAppEnvironment{}, fmt.Errorf("parsing Container App Environment preflight: %w", err)
	}

	return environment, nil
}

// getContainerApp fetches and parses the live Container App definition.
func getContainerApp(ctx context.Context, subscriptionID, resourceGroup, name string) (containerApp, error) {
	res, err := e2eshared.RunWithRetries(ctx, sharedCfg, 3, 5*time.Second,
		"containerapp", "show",
		"--subscription", subscriptionID,
		"--resource-group", resourceGroup,
		"--name", name,
		"--output", "json",
		"--only-show-errors", // suppress the extension-altered-behavior warning that would corrupt JSON
	)
	if err != nil {
		return containerApp{}, err
	}

	var app containerApp
	if err := json.Unmarshal([]byte(res.Stdout), &app); err != nil {
		return containerApp{}, fmt.Errorf("parsing az containerapp show output: %w", err)
	}

	return app, nil
}

// deleteContainerApp catches resources Azure accepted but Terraform never recorded in
// state, such as a create that timed out while Azure continued provisioning it.
func deleteContainerApp(ctx context.Context, subscriptionID, resourceGroup, name string) error {
	_, err := e2eshared.RunWithRetries(ctx, sharedCfg, 3, 5*time.Second,
		"containerapp", "delete",
		"--subscription", subscriptionID,
		"--resource-group", resourceGroup,
		"--name", name,
		"--yes",
		"--no-wait",
		"--only-show-errors",
	)
	if err != nil && !isContainerAppNotFound(err) {
		return err
	}

	return nil
}

func isContainerAppNotFound(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return strings.Contains(message, "ResourceNotFound") || strings.Contains(message, "ContainerAppNotFound")
}

func (a containerApp) sidecar() *caContainer {
	for i := range a.Properties.Template.Containers {
		if a.Properties.Template.Containers[i].Name == sidecarName {
			return &a.Properties.Template.Containers[i]
		}
	}

	return nil
}

func (a containerApp) appContainers() []caContainer {
	var out []caContainer
	for _, c := range a.Properties.Template.Containers {
		if c.Name != sidecarName {
			out = append(out, c)
		}
	}

	return out
}

func (a containerApp) volume(name string) *caVolume {
	for i := range a.Properties.Template.Volumes {
		if a.Properties.Template.Volumes[i].Name == name {
			return &a.Properties.Template.Volumes[i]
		}
	}

	return nil
}

func (a containerApp) hasSecret(name string) bool {
	for _, s := range a.Properties.Configuration.Secrets {
		if s.Name == name {
			return true
		}
	}

	return false
}

func (c caContainer) envVar(name string) *caEnvVar {
	for i := range c.Env {
		if c.Env[i].Name == name {
			return &c.Env[i]
		}
	}

	return nil
}

func (c caContainer) mounts(volumeName string) bool {
	for _, m := range c.VolumeMounts {
		if m.VolumeName == volumeName {
			return true
		}
	}

	return false
}

// envMap flattens a container's env vars into a map for the shared primitives.
func (c caContainer) envMap() map[string]string {
	m := make(map[string]string, len(c.Env))
	for _, e := range c.Env {
		m[e.Name] = e.Value
	}

	return m
}

// verifyInstrumented asserts the instrumented config: sidecar (pinned image), shared
// volume + mounts, required DD_* env vars, the API-key secret, and unified-service-tag
// identity. It asserts identity (values match this run), not mere existence.
func verifyInstrumented(app containerApp, exp Expectations) error {
	var v e2eshared.Violations

	// Sidecar present exactly once, running the pinned serverless-init image.
	sidecarCount := 0
	for _, container := range app.Properties.Template.Containers {
		if container.Name == sidecarName {
			sidecarCount++
		}
	}
	if sidecarCount != 1 {
		v.Addf("%q container count = %d, want 1", sidecarName, sidecarCount)
	}
	sidecar := app.sidecar()
	if sidecar != nil {
		if !strings.Contains(sidecar.Image, serverlessInitRef) {
			v.Addf("sidecar should run serverless-init, got %q", sidecar.Image)
		}
		if sidecar.Image != exp.SidecarImage {
			v.Addf("sidecar image = %q, want pinned %q", sidecar.Image, exp.SidecarImage)
		}
		if !sidecar.mounts(sharedVolumeName) {
			v.Addf("container %q should mount the shared volume", sidecarName)
		}
		e2eshared.RequireValues(&v, "sidecar env var", sidecar.envMap(), map[string]string{
			"DD_ENV":                 exp.Env,
			"DD_SERVERLESS_LOG_PATH": logPath,
			"DD_SERVICE":             exp.Service,
			"DD_SITE":                exp.Site,
			"DD_TAGS":                exp.RunTag,
			"DD_VERSION":             exp.Version,
		})
		switch apiKey := sidecar.envVar("DD_API_KEY"); {
		case apiKey == nil:
			v.Addf("missing sidecar env var DD_API_KEY")
		case apiKey.SecretRef != apiKeySecretName:
			v.Addf("sidecar env var DD_API_KEY secret ref = %q, want %q", apiKey.SecretRef, apiKeySecretName)
		}
	}

	// Shared volume (EmptyDir) plus a mount on every app container.
	switch shared := app.volume(sharedVolumeName); {
	case shared == nil:
		v.Addf("expected a %q volume", sharedVolumeName)
	case shared.StorageType != "EmptyDir":
		v.Addf("shared volume StorageType = %q, want EmptyDir", shared.StorageType)
	}

	appContainers := app.appContainers()
	if len(appContainers) != 1 {
		v.Addf("app container count = %d, want 1", len(appContainers))
	}
	for _, c := range appContainers {
		if c.Name != workloadName {
			v.Addf("app container name = %q, want %q", c.Name, workloadName)
		}
		if c.Image != exp.WorkloadImage {
			v.Addf("container %q image = %q, want pinned %q", c.Name, c.Image, exp.WorkloadImage)
		}
		if !c.mounts(sharedVolumeName) {
			v.Addf("container %q should mount the shared volume", c.Name)
		}
		e2eshared.RequireValues(&v, fmt.Sprintf("container %q env var", c.Name), c.envMap(), map[string]string{
			"DD_ENV":                 exp.Env,
			"DD_LOGS_INJECTION":      "true",
			"DD_SERVERLESS_LOG_PATH": logPath,
			"DD_SERVICE":             exp.Service,
			"DD_TAGS":                exp.RunTag,
			"DD_VERSION":             exp.Version,
		})
	}

	// API-key secret wired.
	if !app.hasSecret(apiKeySecretName) {
		v.Addf("expected the %q secret", apiKeySecretName)
	}

	// Unified service tagging identity on the resource tags + module marker.
	e2eshared.RequireValues(&v, "tag", app.Tags, map[string]string{
		"env":             exp.Env,
		"one_e2e_created": exp.CreatedTS,
		"one_e2e_run_id":  exp.RunID,
		"service":         exp.Service,
		"version":         exp.Version,
	})
	if _, ok := app.Tags[moduleMarkerTag]; !ok {
		v.Addf("module marker tag %q should be present", moduleMarkerTag)
	}

	return v.Err("instrumented contract violated")
}

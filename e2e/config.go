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

// Subset of `az containerapp show` JSON that the conformance contract cares about.
type (
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
	sharedVolumeName  = "shared-volume"
	apiKeySecretName  = "dd-api-key"
	moduleMarkerTag   = "dd_sls_terraform_module"
	serverlessInitRef = "serverless-init"
)

// Expectations pins what an instrumented workload must look like, so a mismatch blames
// the module wiring rather than upstream drift.
type Expectations struct {
	Service      string
	Env          string
	Version      string
	RunID        string
	CreatedTS    string
	SidecarImage string
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

	// Sidecar present, running the pinned serverless-init image.
	sidecar := app.sidecar()
	switch {
	case sidecar == nil:
		v.Addf("expected a %q container", sidecarName)
	default:
		if !strings.Contains(sidecar.Image, serverlessInitRef) {
			v.Addf("sidecar should run serverless-init, got %q", sidecar.Image)
		}
		if sidecar.Image != exp.SidecarImage {
			v.Addf("sidecar image = %q, want pinned %q", sidecar.Image, exp.SidecarImage)
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
	if len(appContainers) == 0 {
		v.Addf("expected at least one app container besides the sidecar")
	}
	for _, c := range appContainers {
		if !c.mounts(sharedVolumeName) {
			v.Addf("container %q should mount the shared volume", c.Name)
		}
		env := c.envMap()
		e2eshared.RequirePresent(&v, "env var", env, "DD_LOGS_INJECTION", "DD_SERVICE", "DD_SERVERLESS_LOG_PATH")
		e2eshared.RequireValues(&v, fmt.Sprintf("container %q env var", c.Name), env, map[string]string{
			"DD_SERVICE": exp.Service,
		})
	}

	// API-key secret wired.
	if !app.hasSecret(apiKeySecretName) {
		v.Addf("expected the %q secret", apiKeySecretName)
	}

	// Unified service tagging identity on the resource tags + module marker.
	e2eshared.RequireValues(&v, "tag", app.Tags, map[string]string{
		"service": exp.Service,
		"env":     exp.Env,
		"version": exp.Version,
	})
	if _, ok := app.Tags[moduleMarkerTag]; !ok {
		v.Addf("module marker tag %q should be present", moduleMarkerTag)
	}
	e2eshared.RequireHygieneTags(&v, sharedCfg, app.Tags, exp.RunID)

	return v.Err("instrumented contract violated")
}

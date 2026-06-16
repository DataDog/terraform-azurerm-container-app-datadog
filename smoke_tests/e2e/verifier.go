// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

func getContainerApp(t *testing.T, subscriptionID, resourceGroup, name string) containerApp {
	t.Helper()
	out, err := runCommandWithRetries(t, 3, 5*time.Second, "az", "containerapp", "show",
		"--subscription", subscriptionID,
		"--resource-group", resourceGroup,
		"--name", name,
		"--output", "json",
	)
	require.NoErrorf(t, err, "az containerapp show failed: %s", out)

	var app containerApp
	require.NoError(t, json.Unmarshal([]byte(out), &app), "parsing az containerapp show output")

	return app
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

func (c caContainer) envValue(name string) (string, bool) {
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value, true
		}
	}

	return "", false
}

// verifyInstrumented asserts the instrumented config: sidecar (pinned image), shared
// volume + mounts, required DD_* env vars, the API-key secret, and unified-service-tag
// identity. It asserts identity (values match this run), not mere existence.
func verifyInstrumented(t *testing.T, app containerApp, want identity) {
	t.Helper()

	// Sidecar present, running the pinned serverless-init image.
	sidecar := app.sidecar()
	require.NotNil(t, sidecar, "expected a %q container", sidecarName)
	require.Contains(t, sidecar.Image, serverlessInitRef, "sidecar should run serverless-init")
	require.Equal(t, want.sidecarImage, sidecar.Image, "sidecar image should be the pinned artifact")

	// Shared volume (EmptyDir) plus a mount on every app container.
	shared := app.volume(sharedVolumeName)
	require.NotNil(t, shared, "expected a %q volume", sharedVolumeName)
	require.Equal(t, "EmptyDir", shared.StorageType, "shared volume should be EmptyDir")

	appContainers := app.appContainers()
	require.NotEmpty(t, appContainers, "expected at least one app container besides the sidecar")
	for _, c := range appContainers {
		require.Truef(t, c.mounts(sharedVolumeName), "container %q should mount the shared volume", c.Name)

		for _, required := range []string{"DD_LOGS_INJECTION", "DD_SERVICE", "DD_SERVERLESS_LOG_PATH"} {
			_, ok := c.envValue(required)
			require.Truef(t, ok, "container %q missing %s", c.Name, required)
		}
		svc, _ := c.envValue("DD_SERVICE")
		require.Equalf(t, want.service, svc, "DD_SERVICE identity on container %q", c.Name)
	}

	// API-key secret wired.
	require.True(t, app.hasSecret(apiKeySecretName), "expected the %q secret", apiKeySecretName)

	// Unified service tagging identity on the resource tags.
	require.Equal(t, want.service, app.Tags["service"], "service tag identity")
	require.Equal(t, want.env, app.Tags["env"], "env tag identity")
	require.Equal(t, want.version, app.Tags["version"], "version tag identity")
	require.Contains(t, app.Tags, moduleMarkerTag, "module marker tag should be present")
	require.Equal(t, want.createdTS, app.Tags[createdTagKey], "freshness tag identity")
}

// verifyUninstrumented asserts the clean end-state: no sidecar, no shared volume, no
// DD_* env vars, no API-key secret, and no DD identity tags. Absence is asserted
// explicitly. The one_e2e_created freshness tag (ours, not Datadog's) may remain.
func verifyUninstrumented(t *testing.T, app containerApp) {
	t.Helper()

	require.Nil(t, app.sidecar(), "sidecar %q should be gone", sidecarName)
	require.Nil(t, app.volume(sharedVolumeName), "shared volume should be gone")

	for _, c := range app.Properties.Template.Containers {
		for _, e := range c.Env {
			require.Falsef(t, strings.HasPrefix(e.Name, "DD_"), "container %q still has DD_ env var %q", c.Name, e.Name)
		}
	}
	require.False(t, app.hasSecret(apiKeySecretName), "%q secret should be gone", apiKeySecretName)

	for _, tag := range []string{moduleMarkerTag, "service", "env", "version"} {
		require.NotContainsf(t, app.Tags, tag, "DD identity tag %q should be gone", tag)
	}
}

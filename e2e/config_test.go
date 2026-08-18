// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsContainerAppNotFound(t *testing.T) {
	require.True(t, isContainerAppNotFound(errors.New("ResourceNotFound")))
	require.True(t, isContainerAppNotFound(errors.New("ContainerAppNotFound")))
	require.False(t, isContainerAppNotFound(errors.New("Forbidden")))
	require.False(t, isContainerAppNotFound(nil))
}

func TestVerifyInstrumented(t *testing.T) {
	exp := expectations{
		Service:       "one-e2e-tf-capp-deadbeef",
		Env:           "e2e",
		Version:       "1.0.0",
		RunID:         "deadbeef",
		RunTag:        "one_e2e_run_id:deadbeef",
		Site:          "datadoghq.com",
		WorkloadImage: "workload@sha256:123",
		SidecarImage:  "datadog/serverless-init@sha256:456",
	}

	t.Run("accepts exact baseline contract", func(t *testing.T) {
		require.NoError(t, verifyInstrumented(instrumentedApp(exp), exp))
	})

	ssiExp := exp
	ssiExp.Instrumentation = &instrumentationExpectations{
		Language:    "python",
		TracerImage: "datadoghq.azurecr.io/dd-lib-python-init:latest",
		LoaderEnv:   map[string]string{"PYTHONPATH": tracerMountPath},
	}
	t.Run("accepts exact SSI contract", func(t *testing.T) {
		require.NoError(t, verifyInstrumented(instrumentedApp(ssiExp), ssiExp))
	})

	tests := []struct {
		name   string
		change func(*containerApp)
		want   string
	}{
		{
			name: "rejects mutable workload",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[1].Image = "workload:latest"
			},
			want: "want configured",
		},
		{
			name: "rejects wrong site",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[0].envVar("DD_SITE").Value = "datadoghq.eu"
			},
			want: "DD_SITE",
		},
		{
			name: "rejects wrong run tag",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[1].envVar("DD_TAGS").Value = "one_e2e_run_id:other"
			},
			want: "DD_TAGS",
		},
		{
			name: "rejects missing freshness tag",
			change: func(app *containerApp) {
				delete(app.Tags, "one_e2e_created")
			},
			want: "one_e2e_created",
		},
		{
			name: "rejects SSI environment on baseline",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[1].Env = append(
					app.Properties.Template.Containers[1].Env,
					caEnvVar{Name: "DD_TRACE_ENABLED", Value: "true"},
				)
			},
			want: "without SSI",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := instrumentedApp(exp)
			tt.change(&app)
			require.ErrorContains(t, verifyInstrumented(app, exp), tt.want)
		})
	}

	ssiTests := []struct {
		name   string
		change func(*containerApp)
		want   string
	}{
		{
			name: "rejects wrong tracer image",
			change: func(app *containerApp) {
				app.Properties.Template.InitContainers[0].Image = "wrong:latest"
			},
			want: "init container image",
		},
		{
			name: "rejects missing loader environment",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[1].Env = withoutEnv(app.Properties.Template.Containers[1].Env, "PYTHONPATH")
			},
			want: "PYTHONPATH",
		},
		{
			name: "rejects tracer mount on sidecar",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[0].VolumeMounts = append(
					app.Properties.Template.Containers[0].VolumeMounts,
					caVolumeMount{VolumeName: tracerVolumeName},
				)
			},
			want: "sidecar should not mount",
		},
		{
			name: "rejects missing adoption tag",
			change: func(app *containerApp) {
				delete(app.Tags, injectionModeTagKey)
			},
			want: injectionModeTagKey,
		},
		{
			name: "rejects duplicate target tracer mount",
			change: func(app *containerApp) {
				app.Properties.Template.Containers[1].VolumeMounts = append(
					app.Properties.Template.Containers[1].VolumeMounts,
					caVolumeMount{VolumeName: tracerVolumeName, MountPath: "/other"},
				)
			},
			want: "mount count",
		},
	}
	for _, tt := range ssiTests {
		t.Run(tt.name, func(t *testing.T) {
			app := instrumentedApp(ssiExp)
			tt.change(&app)
			require.ErrorContains(t, verifyInstrumented(app, ssiExp), tt.want)
		})
	}
}

func instrumentedApp(exp expectations) containerApp {
	env := func(values map[string]string) []caEnvVar {
		out := make([]caEnvVar, 0, len(values))
		for name, value := range values {
			out = append(out, caEnvVar{Name: name, Value: value})
		}
		return out
	}
	common := map[string]string{
		"DD_ENV":                 exp.Env,
		"DD_SERVERLESS_LOG_PATH": logPath,
		"DD_SERVICE":             exp.Service,
		"DD_TAGS":                exp.RunTag,
		"DD_VERSION":             exp.Version,
	}
	sidecarEnv := env(common)
	sidecarEnv = append(sidecarEnv,
		caEnvVar{Name: "DD_SITE", Value: exp.Site},
		caEnvVar{Name: "DD_API_KEY", SecretRef: apiKeySecretName},
	)
	workloadEnv := env(common)
	workloadEnv = append(workloadEnv, caEnvVar{Name: "DD_LOGS_INJECTION", Value: "true"})
	workloadMounts := []caVolumeMount{{VolumeName: sharedVolumeName}}
	if inst := exp.Instrumentation; inst != nil {
		workloadEnv = append(workloadEnv, caEnvVar{Name: "DD_TRACE_ENABLED", Value: "true"})
		for i := range workloadEnv {
			if workloadEnv[i].Name == "DD_TAGS" {
				workloadEnv[i].Value = injectionModeDDTag + "," + exp.RunTag
			}
		}
		for name, value := range inst.LoaderEnv {
			workloadEnv = append(workloadEnv, caEnvVar{Name: name, Value: value})
		}
		workloadMounts = append(workloadMounts, caVolumeMount{VolumeName: tracerVolumeName, MountPath: tracerMountPath})
	}

	var app containerApp
	app.Properties.Template.Containers = []caContainer{
		{
			Name:         sidecarName,
			Image:        exp.SidecarImage,
			Env:          sidecarEnv,
			VolumeMounts: []caVolumeMount{{VolumeName: sharedVolumeName}},
		},
		{
			Name:         workloadName,
			Image:        exp.WorkloadImage,
			Env:          workloadEnv,
			VolumeMounts: workloadMounts,
		},
	}
	app.Properties.Template.Volumes = []caVolume{{Name: sharedVolumeName, StorageType: "EmptyDir"}}
	if inst := exp.Instrumentation; inst != nil {
		app.Properties.Template.InitContainers = []caContainer{{
			Name:         tracerInitName,
			Image:        inst.TracerImage,
			Command:      []string{"/datadog-init/copy-lib.sh"},
			Args:         []string{tracerMountPath},
			VolumeMounts: []caVolumeMount{{VolumeName: tracerVolumeName, MountPath: tracerMountPath}},
		}}
		app.Properties.Template.Volumes = append(app.Properties.Template.Volumes, caVolume{
			Name: tracerVolumeName, StorageType: "EmptyDir",
		})
	}
	app.Properties.Configuration.Secrets = append(app.Properties.Configuration.Secrets, struct {
		Name string `json:"name"`
	}{Name: apiKeySecretName})
	app.Tags = map[string]string{
		"dd_sls_terraform_module": "1.2.0",
		"env":                     exp.Env,
		"one_e2e_created":         "1234567890",
		"one_e2e_run_id":          exp.RunID,
		"service":                 exp.Service,
		"version":                 exp.Version,
	}
	if exp.Instrumentation != nil {
		app.Tags[injectionModeTagKey] = injectionModeTagValue
	}

	return app
}

func withoutEnv(env []caEnvVar, name string) []caEnvVar {
	var out []caEnvVar
	for _, item := range env {
		if item.Name != name {
			out = append(out, item)
		}
	}

	return out
}

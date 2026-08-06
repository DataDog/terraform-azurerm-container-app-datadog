// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyInstrumented(t *testing.T) {
	exp := Expectations{
		Service:       "one-e2e-tf-capp-deadbeef",
		Env:           "e2e",
		Version:       "1.0.0",
		RunID:         "deadbeef",
		RunTag:        "one_e2e_run_id:deadbeef",
		CreatedTS:     "1234567890",
		Site:          "datadoghq.com",
		WorkloadImage: "workload@sha256:123",
		SidecarImage:  "datadog/serverless-init@sha256:456",
	}

	t.Run("accepts exact contract", func(t *testing.T) {
		require.NoError(t, verifyInstrumented(instrumentedApp(exp), exp))
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
			want: "want pinned",
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
			name: "rejects wrong freshness tag",
			change: func(app *containerApp) {
				app.Tags["one_e2e_created"] = "1"
			},
			want: "one_e2e_created",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := instrumentedApp(exp)
			tt.change(&app)
			require.ErrorContains(t, verifyInstrumented(app, exp), tt.want)
		})
	}
}

func instrumentedApp(exp Expectations) containerApp {
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
			VolumeMounts: []caVolumeMount{{VolumeName: sharedVolumeName}},
		},
	}
	app.Properties.Template.Volumes = []caVolume{{Name: sharedVolumeName, StorageType: "EmptyDir"}}
	app.Properties.Configuration.Secrets = append(app.Properties.Configuration.Secrets, struct {
		Name string `json:"name"`
	}{Name: apiKeySecretName})
	app.Tags = map[string]string{
		"dd_sls_terraform_module": "1.2.0",
		"env":                     exp.Env,
		"one_e2e_created":         exp.CreatedTS,
		"one_e2e_run_id":          exp.RunID,
		"service":                 exp.Service,
		"version":                 exp.Version,
	}

	return app
}

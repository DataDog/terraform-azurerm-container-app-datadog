// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/stretchr/testify/require"
)

const (
	pollInterval = 15 * time.Second
	maxAttempts  = 20
	lookback     = 15 * time.Minute
)

// identity is the run's fingerprint, applied to config and propagated onto ingested
// telemetry. Telemetry assertions filter on these so a match proves identity, not
// the mere existence of unrelated telemetry.
type identity struct {
	service      string
	env          string
	version      string
	runTag       string // one_e2e_run_id:<runid>
	createdTS    string
	sidecarImage string
}

// telemetryQuery builds a filter that pins every identifying facet at once: service,
// env, version, and the unique run-id tag. Anything it returns is provably this run's.
func (id identity) telemetryQuery() string {
	return fmt.Sprintf("service:%s env:%s version:%s %s", id.service, id.env, id.version, id.runTag)
}

func datadogContext(apiKey, appKey, site string) context.Context {
	ctx := context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: apiKey},
		"appKeyAuth": {Key: appKey},
	})

	return context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{"site": site})
}

// pollUntilFound polls query on a bounded budget until it returns at least one item.
func pollUntilFound(t *testing.T, label string, query func() (int, error)) {
	t.Helper()
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		t.Logf("[%s] attempt %d/%d", label, attempt, maxAttempts)
		count, err := query()
		if err != nil {
			t.Logf("[%s] query error: %v", label, err)
		} else if count > 0 {
			t.Logf("[%s] found %d item(s)", label, count)

			return
		}
		if attempt < maxAttempts {
			time.Sleep(pollInterval)
		}
	}
	require.Failf(t, "telemetry not found",
		"[%s] timed out after %d attempts (%s)", label, maxAttempts, time.Duration(maxAttempts)*pollInterval)
}

// checkTelemetryFlowing asserts that both traces and logs carrying this run's identity
// reach Datadog. Spans and logs are polled concurrently on the same budget.
func checkTelemetryFlowing(t *testing.T, apiKey, appKey, site string, id identity) {
	t.Helper()
	ctx := datadogContext(apiKey, appKey, site)
	client := datadog.NewAPIClient(datadog.NewConfiguration())
	query := id.telemetryQuery()
	t.Logf("polling Datadog (%s) for telemetry matching: %s", site, query)

	spansAPI := datadogV2.NewSpansApi(client)
	logsAPI := datadogV2.NewLogsApi(client)

	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		pollUntilFound(t, "spans", func() (int, error) { return querySpans(ctx, spansAPI, query) })
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		pollUntilFound(t, "logs", func() (int, error) { return queryLogs(ctx, logsAPI, query) })
	}()
	<-done
	<-done
}

func querySpans(ctx context.Context, api *datadogV2.SpansApi, query string) (int, error) {
	now := time.Now()
	body := datadogV2.SpansListRequest{
		Data: &datadogV2.SpansListRequestData{
			Type: datadogV2.SPANSLISTREQUESTTYPE_SEARCH_REQUEST.Ptr(),
			Attributes: &datadogV2.SpansListRequestAttributes{
				Filter: &datadogV2.SpansQueryFilter{
					Query: datadog.PtrString(query),
					From:  datadog.PtrString(now.Add(-lookback).Format(time.RFC3339)),
					To:    datadog.PtrString(now.Format(time.RFC3339)),
				},
				Page: &datadogV2.SpansListRequestPage{Limit: datadog.PtrInt32(5)},
			},
		},
	}
	resp, _, err := api.ListSpans(ctx, body)
	if err != nil {
		return 0, err
	}

	return len(resp.GetData()), nil
}

func queryLogs(ctx context.Context, api *datadogV2.LogsApi, query string) (int, error) {
	now := time.Now()
	body := datadogV2.LogsListRequest{
		Filter: &datadogV2.LogsQueryFilter{
			Query: datadog.PtrString(query),
			From:  datadog.PtrString(now.Add(-lookback).Format(time.RFC3339)),
			To:    datadog.PtrString(now.Format(time.RFC3339)),
		},
		Page: &datadogV2.LogsListRequestPage{Limit: datadog.PtrInt32(5)},
	}
	resp, _, err := api.ListLogs(ctx, *datadogV2.NewListLogsOptionalParameters().WithBody(body))
	if err != nil {
		return 0, err
	}

	return len(resp.GetData()), nil
}

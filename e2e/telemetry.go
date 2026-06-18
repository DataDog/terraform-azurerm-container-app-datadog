// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"context"
	"fmt"
	"strings"
	"time"

	e2eshared "github.com/DataDog/terraform-azurerm-container-app-datadog/e2e/shared"
)

const (
	telemetryPollInterval = 15 * time.Second
	telemetryMaxAttempts  = 24 // 6 min; spans index slower than logs and an occasional 429 costs an attempt
)

// telemetryIdentity is the reduced fingerprint this module asserts on ingested
// telemetry: the unique service, env, and the unique run-id tag. version is deliberately
// omitted -- the resource/env-var version is asserted in config verification, but whether
// the per-runtime tracer stamps `version` onto spans is upstream-owned (the spec scopes
// tracer tag propagation out), and the Node tracer does not, so requiring it here would
// assert an upstream behavior, not the module's wiring.
type telemetryIdentity struct {
	service string
	env     string
	runTag  string // one_e2e_run_id:<runid>
}

// query pins the identifying facets that ride onto both traces and logs. Anything it
// returns is provably this run's telemetry (assert identity, not existence).
func (id telemetryIdentity) query() string {
	return fmt.Sprintf("service:%s env:%s %s", id.service, id.env, id.runTag)
}

// matches reports whether an ingested event carries this run's reduced identity.
func (id telemetryIdentity) matches(e e2eshared.Event) bool {
	tagKey, tagVal, _ := strings.Cut(id.runTag, ":")

	return e.Has("service", id.service) && e.Has("env", id.env) && e.Has(tagKey, tagVal)
}

// waitForTelemetry polls a search function on a bounded budget until at least one event
// carries the reduced identity. It retries the cloud (transient query errors, propagation
// delay) but never declares success without a match.
func waitForTelemetry(
	ctx context.Context,
	label string,
	search func(context.Context, string) ([]e2eshared.Event, error),
	id telemetryIdentity,
) error {
	query := id.query()
	var lastErr error
	for attempt := 1; attempt <= telemetryMaxAttempts; attempt++ {
		events, err := search(ctx, query)
		if err != nil {
			lastErr = err
		} else {
			for _, e := range events {
				if id.matches(e) {
					return nil
				}
			}
			if len(events) > 0 {
				lastErr = fmt.Errorf("%d %s found for query %q but none carried the expected identity %+v", len(events), label, query, id)
			} else {
				lastErr = fmt.Errorf("no %s found yet for query %q", label, query)
			}
		}
		if attempt < telemetryMaxAttempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(telemetryPollInterval):
			}
		}
	}

	return fmt.Errorf("[%s] timed out after %d attempts (%s): %w",
		label, telemetryMaxAttempts, time.Duration(telemetryMaxAttempts)*telemetryPollInterval, lastErr)
}

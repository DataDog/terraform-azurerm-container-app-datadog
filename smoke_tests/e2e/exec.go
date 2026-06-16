// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// retryablePatterns are transient cloud-provider errors that are safe to retry.
// We retry the cloud, not the assertions: never retry past a real failure.
var retryablePatterns = []string{
	"GatewayTimeout",
	"TooManyRequests",
	"Conflict", // a stale revision/name still being deleted during an instrument<->plain flip
	"OperationNotAllowed",
	"ETIMEDOUT",
	"ECONNRESET",
	"temporarily unavailable",
	"ServiceUnavailable",
	"InternalServerError",
}

func isRetryable(output string) bool {
	for _, p := range retryablePatterns {
		if strings.Contains(output, p) {
			return true
		}
	}

	return false
}

// runCommand runs a command once and returns combined output.
func runCommand(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()

	return string(out), err
}

// runCommandWithRetries runs a command, retrying only on transient cloud errors.
func runCommandWithRetries(t *testing.T, maxAttempts int, delay time.Duration, name string, args ...string) (string, error) {
	t.Helper()
	var (
		out string
		err error
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		out, err = runCommand(name, args...)
		if err == nil {
			return out, nil
		}
		if attempt < maxAttempts && isRetryable(out) {
			t.Logf("command %q failed with retryable error (attempt %d/%d), retrying in %s\n%s",
				name, attempt, maxAttempts, delay, out)
			time.Sleep(delay)

			continue
		}

		break
	}

	return out, err
}

// retryableTerraformErrors are surfaced to Terratest so it retries apply/destroy on
// the same transient conditions (notably the brief name Conflict during a flip).
var retryableTerraformErrors = map[string]string{
	".*Conflict.*":                "Resource conflict, likely a delete still in flight; retrying.",
	".*TooManyRequests.*":         "Azure throttling; retrying.",
	".*GatewayTimeout.*":          "Azure gateway timeout; retrying.",
	".*ServiceUnavailable.*":      "Azure service unavailable; retrying.",
	".*InternalServerError.*":     "Azure internal error; retrying.",
	".*temporarily unavailable.*": "Transient unavailability; retrying.",
}

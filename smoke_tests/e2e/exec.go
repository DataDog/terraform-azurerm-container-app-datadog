// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"bytes"
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

// runCommand runs a command once, returning stdout and stderr separately so callers
// that parse stdout (e.g. JSON) aren't corrupted by warnings the CLI writes to stderr.
func runCommand(name string, args ...string) (stdout, stderr string, err error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()

	return outBuf.String(), errBuf.String(), err
}

// runCommandWithRetries runs a command, retrying only on transient cloud errors, and
// returns stdout. The retry decision considers both streams.
func runCommandWithRetries(t *testing.T, maxAttempts int, delay time.Duration, name string, args ...string) (string, error) {
	t.Helper()
	var (
		stdout string
		stderr string
		err    error
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		stdout, stderr, err = runCommand(name, args...)
		if err == nil {
			return stdout, nil
		}
		if attempt < maxAttempts && isRetryable(stdout+stderr) {
			t.Logf("command %q failed with retryable error (attempt %d/%d), retrying in %s\n%s",
				name, attempt, maxAttempts, delay, stderr)
			time.Sleep(delay)

			continue
		}

		break
	}

	return stdout, err
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

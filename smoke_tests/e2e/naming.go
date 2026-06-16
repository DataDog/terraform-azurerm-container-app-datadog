// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

// Resource hygiene convention (shared across the serverless e2e suites):
//
//	name prefix  one-e2e-<tool>-<platform>-<runid>
//	freshness tag one_e2e_created:<unix-ts>
//
// The prefix is the sweeper's blast-radius guard, set atomically at creation. Azure
// Container App names are capped at 32 chars (the tightest budget across platforms),
// so the tool/platform tokens are kept short: one-e2e-tf-capp-<8 hex> == 24 chars.
const (
	namePrefix      = "one-e2e"
	toolToken       = "tf"   // this repo: a Terraform module
	platformToken   = "capp" // Azure Container App
	runIDTagKey     = "one_e2e_run_id"
	createdTagKey   = "one_e2e_created"
	runIDByteLength = 4
)

// newRunID returns a short, unique-per-run hex token.
func newRunID() string {
	b := make([]byte, runIDByteLength)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail; fall back to the clock so a run can proceed.
		return strconv.FormatInt(time.Now().UnixNano(), 16)
	}

	return hex.EncodeToString(b)
}

// appName builds the hygiene-compliant Container App name for a run.
func appName(runID string) string {
	return fmt.Sprintf("%s-%s-%s-%s", namePrefix, toolToken, platformToken, runID)
}

// runIDTag is the unique run-id marker propagated to telemetry via DD_TAGS.
func runIDTag(runID string) string {
	return fmt.Sprintf("%s:%s", runIDTagKey, runID)
}

// createdTimestamp is the value of the one_e2e_created freshness tag, set at creation.
// Native creation time isn't usable cross-cloud, so the sweeper relies on this tag.
func createdTimestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

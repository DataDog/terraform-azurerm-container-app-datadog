// Unless explicitly stated otherwise all files in this repository are licensed under the Apache-2.0 License.
// This product includes software developed at Datadog (https://www.datadoghq.com/) Copyright 2026 Datadog, Inc.

package e2e

import (
	"sync"
	"testing"
	"time"
)

const phaseHeartbeatInterval = 30 * time.Second

func runPhase(t *testing.T, name string, run func()) {
	t.Helper()
	started := time.Now()
	t.Logf("[phase] START %s", name)

	stop := make(chan struct{})
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		ticker := time.NewTicker(phaseHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.Logf("[phase] RUNNING %s elapsed=%s", name, time.Since(started).Round(time.Second))
			case <-stop:
				return
			}
		}
	}()

	defer func() {
		close(stop)
		heartbeat.Wait()
		t.Logf("[phase] DONE %s elapsed=%s", name, time.Since(started).Round(time.Millisecond))
	}()

	run()
}

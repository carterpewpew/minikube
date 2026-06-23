//go:build integration

/*
Copyright 2026 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package integration

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestMDNS tests that the --mdns flag enables mDNS on the VM and that the
// setting persists across a stop/start cycle (it is baked into cluster config).
func TestMDNS(t *testing.T) {
	if !VMDriver() {
		t.Skip("--mdns is only supported with VM drivers")
	}
	MaybeParallel(t)

	t.Run("Enabled", testMDNSEnabled)
	t.Run("Disabled", testMDNSDisabled)
}

func testMDNSEnabled(t *testing.T) {
	MaybeParallel(t)
	profile := UniqueProfileName("mdns-on")
	ctx, cancel := context.WithTimeout(t.Context(), Minutes(12))
	defer CleanupWithLogs(t, profile, cancel)

	// 1. Start with --mdns
	startCtx, startCancel := context.WithTimeout(ctx, Minutes(5))
	defer startCancel()
	startArgs := append([]string{"start", "-p", profile, "--mdns", "--memory=3072"}, StartArgs()...)
	if _, err := Run(t, exec.CommandContext(startCtx, Target(), startArgs...)); err != nil {
		t.Fatalf("failed to start minikube: %v", err)
	}

	// 2. Validate mDNS is enabled
	assertMDNS(ctx, t, profile, true)

	// 3. Stop
	stopCtx, stopCancel := context.WithTimeout(ctx, Minutes(1))
	defer stopCancel()
	if _, err := Run(t, exec.CommandContext(stopCtx, Target(), "stop", "-p", profile)); err != nil {
		t.Fatalf("failed to stop minikube: %v", err)
	}

	// 4. Start without --mdns (setting persists in cluster config)
	restartCtx, restartCancel := context.WithTimeout(ctx, Minutes(5))
	defer restartCancel()
	restartArgs := append([]string{"start", "-p", profile}, StartArgs()...)
	if _, err := Run(t, exec.CommandContext(restartCtx, Target(), restartArgs...)); err != nil {
		t.Fatalf("failed to restart minikube: %v", err)
	}

	// 5. Validate mDNS is still enabled
	assertMDNS(ctx, t, profile, true)
}

func testMDNSDisabled(t *testing.T) {
	MaybeParallel(t)
	profile := UniqueProfileName("mdns-off")
	ctx, cancel := context.WithTimeout(t.Context(), Minutes(7))
	defer CleanupWithLogs(t, profile, cancel)

	// Start without --mdns
	startCtx, startCancel := context.WithTimeout(ctx, Minutes(5))
	defer startCancel()
	startArgs := append([]string{"start", "-p", profile, "--memory=3072"}, StartArgs()...)
	if _, err := Run(t, exec.CommandContext(startCtx, Target(), startArgs...)); err != nil {
		t.Fatalf("failed to start minikube: %v", err)
	}

	// Validate mDNS is NOT enabled
	assertMDNS(ctx, t, profile, false)
}

// TestDNSServers tests that the --dns-servers flag configures static DNS
// servers on the VM and that the setting persists across stop/start.
func TestDNSServers(t *testing.T) {
	if !VMDriver() {
		t.Skip("--dns-servers is only supported with VM drivers")
	}
	MaybeParallel(t)

	profile := UniqueProfileName("dns-srv")
	ctx, cancel := context.WithTimeout(t.Context(), Minutes(12))
	defer CleanupWithLogs(t, profile, cancel)

	// 1. Start with --dns-servers
	startCtx, startCancel := context.WithTimeout(ctx, Minutes(5))
	defer startCancel()
	startArgs := append([]string{"start", "-p", profile, "--dns-servers=8.8.8.8,1.1.1.1", "--memory=3072"}, StartArgs()...)
	if _, err := Run(t, exec.CommandContext(startCtx, Target(), startArgs...)); err != nil {
		t.Fatalf("failed to start minikube: %v", err)
	}

	// 2. Validate DNS servers are configured
	assertDNSServers(ctx, t, profile, []string{"8.8.8.8", "1.1.1.1"})

	// 3. Stop
	stopCtx, stopCancel := context.WithTimeout(ctx, Minutes(1))
	defer stopCancel()
	if _, err := Run(t, exec.CommandContext(stopCtx, Target(), "stop", "-p", profile)); err != nil {
		t.Fatalf("failed to stop minikube: %v", err)
	}

	// 4. Start without --dns-servers (setting persists)
	restartCtx, restartCancel := context.WithTimeout(ctx, Minutes(5))
	defer restartCancel()
	restartArgs := append([]string{"start", "-p", profile}, StartArgs()...)
	if _, err := Run(t, exec.CommandContext(restartCtx, Target(), restartArgs...)); err != nil {
		t.Fatalf("failed to restart minikube: %v", err)
	}

	// 5. Validate DNS servers still configured
	assertDNSServers(ctx, t, profile, []string{"8.8.8.8", "1.1.1.1"})
}

// assertMDNS checks whether mDNS is enabled or disabled on eth0 by
// querying resolvectl status. The output contains "+mDNS" when enabled.
func assertMDNS(ctx context.Context, t *testing.T, profile string, wantEnabled bool) {
	t.Helper()
	rr, err := Run(t, exec.CommandContext(ctx, Target(), "ssh", "-p", profile, "--", "sudo", "resolvectl", "status", "eth0"))
	if err != nil {
		t.Fatalf("failed to query resolvectl status: %v", err)
	}
	hasMDNS := strings.Contains(rr.Output(), "+mDNS")
	if wantEnabled && !hasMDNS {
		t.Errorf("expected mDNS to be enabled (+mDNS) on eth0, got: %s", rr.Output())
	}
	if !wantEnabled && hasMDNS {
		t.Errorf("expected mDNS to be disabled on eth0, but found +mDNS in: %s", rr.Output())
	}
}

// assertDNSServers checks that the expected DNS servers are configured on eth0.
func assertDNSServers(ctx context.Context, t *testing.T, profile string, servers []string) {
	t.Helper()
	rr, err := Run(t, exec.CommandContext(ctx, Target(), "ssh", "-p", profile, "--", "sudo", "resolvectl", "status", "eth0"))
	if err != nil {
		t.Fatalf("failed to query resolvectl status: %v", err)
	}
	output := rr.Output()
	for _, s := range servers {
		if !strings.Contains(output, s) {
			t.Errorf("expected DNS server %s in resolvectl output, got: %s", s, output)
		}
	}
}

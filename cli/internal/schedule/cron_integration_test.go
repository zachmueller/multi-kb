//go:build integration && unix

package schedule

import (
	"testing"
)

func TestCronScheduler_RealCrontab(t *testing.T) {
	sched := NewScheduler()

	binaryPath := "/tmp/multi-kb-integration-test-binary"
	configPath := "/tmp/multi-kb-integration-test-config.yaml"
	cronExpr := "0 */2 * * *"

	// Install
	if err := sched.Install(cronExpr, binaryPath, configPath); err != nil {
		t.Skipf("skipping: cron install failed (may need crontab access): %v", err)
	}

	// Verify installed
	installed, err := sched.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if !installed {
		t.Error("expected cron entry to be installed")
	}

	// Uninstall
	if err := sched.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}

	// Verify removed
	installed, err = sched.IsInstalled()
	if err != nil {
		t.Fatalf("IsInstalled after uninstall: %v", err)
	}
	if installed {
		t.Error("expected cron entry to be removed")
	}
}

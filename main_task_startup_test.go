package main

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMainStartsInvitationRewardEventRetryTask(t *testing.T) {
	content, err := os.ReadFile("main.go")
	require.NoError(t, err)
	source := string(content)

	entitlementCall := "service.StartInvitationEntitlementRefreshTask()"
	retryCall := "service.StartInvitationRewardEventRetryTask()"
	require.Contains(t, source, entitlementCall)
	require.Contains(t, source, retryCall)
	assert.Greater(t, strings.Index(source, retryCall), strings.Index(source, entitlementCall))
}

func TestMainStartsKyrenReconciliationTask(t *testing.T) {
	content, err := os.ReadFile("main.go")
	require.NoError(t, err)
	assert.Contains(t, string(content), "controller.StartKyrenReconciliationTask()")
}

func TestMaintenanceStartupPlanDisablesAllRuntimeStarts(t *testing.T) {
	plan := runtimeStartupPlanFor(true)
	require.True(t, plan.maintenanceMode)
	require.False(t, plan.startApplication)
	require.False(t, plan.startRedis)
	require.False(t, plan.startBackground)
	require.False(t, plan.startSystemMonitor)
	require.False(t, plan.startHTTP)
}

func TestRuntimeStartupPlanForMaintenanceDisablesEveryRuntimeStart(t *testing.T) {
	plan := runtimeStartupPlanFor(true)
	require.True(t, plan.maintenanceMode)
	require.False(t, plan.startApplication)
	require.False(t, plan.startRedis)
	require.False(t, plan.startBackground)
	require.False(t, plan.startSystemMonitor)
	require.False(t, plan.startHTTP)
}

func TestRuntimeStartupPlanForNormalModeEnablesEveryRuntimeStart(t *testing.T) {
	plan := runtimeStartupPlanFor(false)
	require.False(t, plan.maintenanceMode)
	require.True(t, plan.startApplication)
	require.True(t, plan.startRedis)
	require.True(t, plan.startBackground)
	require.True(t, plan.startSystemMonitor)
	require.True(t, plan.startHTTP)
}

func TestMaintenanceWaitBlocksUntilTerminationSignal(t *testing.T) {
	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	go func() {
		waitForMaintenanceShutdown(signals)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("maintenance wait returned before receiving a termination signal")
	case <-time.After(50 * time.Millisecond):
	}

	signals <- os.Interrupt
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("maintenance wait did not return after receiving a termination signal")
	}
}

func TestMaintenanceModeRegistersTerminationSignals(t *testing.T) {
	content, err := os.ReadFile("main.go")
	require.NoError(t, err)
	source := string(content)
	require.Contains(t, source, "signal.Notify(signals, os.Interrupt, syscall.SIGTERM)")
	require.Contains(t, source, "defer signal.Stop(signals)")
	require.Contains(t, source, "blockInMaintenanceMode()")
}
func TestMaintenanceSchemaFailureDoesNotPublishReadiness(t *testing.T) {
	path := t.TempDir() + "/schema-ready"
	closeCalled := false
	err := stageMaintenanceSchema(path, func() error {
		_, statErr := os.Stat(path)
		require.True(t, os.IsNotExist(statErr))
		return errors.New("migration failed")
	}, func() error {
		closeCalled = true
		return nil
	})

	require.ErrorContains(t, err, "migration failed")
	require.True(t, closeCalled)
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr))
}

func TestMaintenanceSchemaReadinessPublishesAfterMigrationAndCleansOnExit(t *testing.T) {
	directory := t.TempDir()
	schemaPath := directory + "/schema-ready"
	runtimePath := directory + "/maintenance-ready"
	migrated := false
	require.NoError(t, stageMaintenanceSchema(schemaPath, func() error {
		_, statErr := os.Stat(schemaPath)
		require.True(t, os.IsNotExist(statErr))
		migrated = true
		return nil
	}, func() error {
		t.Fatal("successful schema staging must not close the maintenance database")
		return nil
	}))
	require.True(t, migrated)
	require.FileExists(t, schemaPath)

	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runMaintenanceModeSession(runtimePath, schemaPath, signals)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, statErr := os.Stat(runtimePath); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maintenance readiness file was not created")
		}
		time.Sleep(10 * time.Millisecond)
	}
	signals <- os.Interrupt
	require.NoError(t, <-done)
	for _, path := range []string{runtimePath, schemaPath} {
		_, statErr := os.Stat(path)
		require.True(t, os.IsNotExist(statErr), "%s was not removed: %v", path, statErr)
	}
}

func TestMaintenanceReadinessFileIsCreatedWithPrivatePermissions(t *testing.T) {
	path := t.TempDir() + "/maintenance-ready"
	require.NoError(t, writeMaintenanceReadiness(path))
	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "ready\n", string(content))
}

func TestNormalStartupPlanDoesNotEnterMaintenanceBranch(t *testing.T) {
	plan := runtimeStartupPlanFor(false)
	require.True(t, plan.startApplication)
	require.False(t, plan.maintenanceMode)

	content, err := os.ReadFile("main.go")
	require.NoError(t, err)
	source := string(content)
	require.Contains(t, source, "if !startupPlan.startApplication {")
	require.Contains(t, source, "blockInMaintenanceMode()")
	require.Contains(t, source, "common.SysLog(\"New API \" + common.Version + \" started\")")
}

func TestMaintenanceSessionCleansReadinessAndClosesNilMaintenanceDatabase(t *testing.T) {
	path := t.TempDir() + "/maintenance-ready"
	signals := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- runMaintenanceSession(path, signals)
	}()

	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maintenance readiness file was not created")
		}
		time.Sleep(10 * time.Millisecond)
	}

	signals <- os.Interrupt
	require.NoError(t, <-done)
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err), "maintenance readiness file was not removed: %v", err)
	require.NoError(t, closeRuntimeDatabases(true))
}

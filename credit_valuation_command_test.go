package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestParseCreditValuationCommandArgsAcceptsAllModes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want CreditValuationCommandOptions
	}{
		{
			name: "dry run",
			args: []string{"--dry-run", "--version", "1"},
			want: CreditValuationCommandOptions{Mode: model.CreditValuationMigrationModeDryRun, Version: 1, BatchSize: CreditValuationCommandDefaultBatchSize},
		},
		{
			name: "apply with explicit batch size",
			args: []string{"--apply", "--version", "2", "--batch-size", "25"},
			want: CreditValuationCommandOptions{Mode: model.CreditValuationMigrationModeApply, Version: 2, BatchSize: 25},
		},
		{
			name: "verify",
			args: []string{"--verify", "--version", "3"},
			want: CreditValuationCommandOptions{Mode: model.CreditValuationMigrationModeVerify, Version: 3, BatchSize: CreditValuationCommandDefaultBatchSize},
		},
		{
			name: "repair missing as unknown",
			args: []string{"--repair-missing-as-unknown", "--version", "4"},
			want: CreditValuationCommandOptions{Mode: model.CreditValuationMigrationModeRepairMissingAsUnknown, Version: 4, BatchSize: CreditValuationCommandDefaultBatchSize},
		},
		{
			name: "suspend with reason",
			args: []string{"--suspend", "--version", "5", "--reason", "planned maintenance"},
			want: CreditValuationCommandOptions{Mode: model.CreditValuationMigrationModeSuspend, Version: 5, BatchSize: CreditValuationCommandDefaultBatchSize, Reason: "planned maintenance"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseCreditValuationCommandArgs(test.args)
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestParseCreditValuationCommandArgsReturnsStableCodes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "mode missing", args: []string{"--version", "1"}, code: CreditValuationCommandCodeModeRequired},
		{name: "mode duplicate", args: []string{"--dry-run", "--dry-run", "--version", "1"}, code: CreditValuationCommandCodeModeDuplicate},
		{name: "mode conflict", args: []string{"--dry-run", "--verify", "--version", "1"}, code: CreditValuationCommandCodeModeConflict},
		{name: "version missing", args: []string{"--dry-run"}, code: CreditValuationCommandCodeVersionRequired},
		{name: "version zero", args: []string{"--dry-run", "--version", "0"}, code: CreditValuationCommandCodeVersionInvalid},
		{name: "version negative", args: []string{"--dry-run", "--version", "-1"}, code: CreditValuationCommandCodeVersionInvalid},
		{name: "version non integer", args: []string{"--dry-run", "--version", "one"}, code: CreditValuationCommandCodeVersionInvalid},
		{name: "version value missing", args: []string{"--dry-run", "--version"}, code: CreditValuationCommandCodeFlagValueRequired},
		{name: "batch zero", args: []string{"--apply", "--version", "1", "--batch-size", "0"}, code: CreditValuationCommandCodeBatchSizeInvalid},
		{name: "batch negative", args: []string{"--apply", "--version", "1", "--batch-size", "-2"}, code: CreditValuationCommandCodeBatchSizeInvalid},
		{name: "batch non integer", args: []string{"--apply", "--version", "1", "--batch-size", "many"}, code: CreditValuationCommandCodeBatchSizeInvalid},
		{name: "batch value missing", args: []string{"--apply", "--version", "1", "--batch-size"}, code: CreditValuationCommandCodeFlagValueRequired},
		{name: "suspend reason omitted", args: []string{"--suspend", "--version", "1"}, code: CreditValuationCommandCodeReasonRequired},
		{name: "suspend reason empty", args: []string{"--suspend", "--version", "1", "--reason", "   "}, code: CreditValuationCommandCodeReasonRequired},
		{name: "reason not allowed", args: []string{"--verify", "--version", "1", "--reason", "maintenance"}, code: CreditValuationCommandCodeReasonNotAllowed},
		{name: "batch not allowed for dry run", args: []string{"--dry-run", "--version", "1", "--batch-size", "10"}, code: CreditValuationCommandCodeBatchSizeNotAllowed},
		{name: "batch not allowed for verify", args: []string{"--verify", "--version", "1", "--batch-size", "10"}, code: CreditValuationCommandCodeBatchSizeNotAllowed},
		{name: "batch not allowed for repair", args: []string{"--repair-missing-as-unknown", "--version", "1", "--batch-size", "10"}, code: CreditValuationCommandCodeBatchSizeNotAllowed},
		{name: "batch not allowed for suspend", args: []string{"--suspend", "--version", "1", "--reason", "maintenance", "--batch-size", "10"}, code: CreditValuationCommandCodeBatchSizeNotAllowed},
		{name: "unknown flag", args: []string{"--dry-run", "--version", "1", "--other"}, code: CreditValuationCommandCodeUnknownFlag},
		{name: "trailing argument", args: []string{"--dry-run", "--version", "1", "trailing"}, code: CreditValuationCommandCodeUnexpectedArgument},
		{name: "duplicate version", args: []string{"--dry-run", "--version", "1", "--version", "2"}, code: CreditValuationCommandCodeFlagDuplicate},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseCreditValuationCommandArgs(test.args)
			require.Error(t, err)
			var commandErr *CreditValuationCommandError
			require.True(t, errors.As(err, &commandErr))
			require.Equal(t, test.code, commandErr.Code)
		})
	}
}

func TestCreditValuationCommandDispatchesBeforeResourceInitialization(t *testing.T) {
	content, err := os.ReadFile("main.go")
	require.NoError(t, err)
	source := string(content)
	commandIndex := strings.Index(source, `os.Args[1] == "credit-valuation-migrate"`)
	resourceIndex := strings.Index(source, "InitResources()")
	require.NotEqual(t, -1, commandIndex)
	require.NotEqual(t, -1, resourceIndex)
	require.Less(t, commandIndex, resourceIndex)
}

func TestRunCreditValuationCommandRejectsArgumentsBeforeOpeningDatabase(t *testing.T) {
	initCalled := false
	runCalled := false
	closeCalled := false
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := runCreditValuationCommand(
		[]string{"--dry-run"},
		&stdout,
		&stderr,
		creditValuationCommandDependencies{
			initMaintenanceDB: func() (*gorm.DB, error) {
				initCalled = true
				return nil, nil
			},
			closeMaintenanceDB: func() error {
				closeCalled = true
				return nil
			},
			runMigration: func(*gorm.DB, model.CreditValuationMigrationRequest) (model.CreditValuationMigrationReport, error) {
				runCalled = true
				return model.CreditValuationMigrationReport{}, nil
			},
		},
	)

	require.NotZero(t, exitCode)
	require.False(t, initCalled)
	require.False(t, runCalled)
	require.False(t, closeCalled)
	require.Empty(t, stdout.String())
	require.Equal(t, 1, strings.Count(stderr.String(), "\n"))

	var output creditValuationCommandErrorOutput
	require.NoError(t, common.Unmarshal(stderr.Bytes(), &output))
	require.False(t, output.Success)
	require.Equal(t, CreditValuationCommandCodeVersionRequired, output.Code)
	require.NotEmpty(t, output.Message)
}

func TestRunCreditValuationCommandPreservesMigrationFailureReport(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	expected := model.CreditValuationMigrationReport{
		Version:  1,
		Mode:     model.CreditValuationMigrationModeApply,
		Status:   model.CreditValuationMigrationFailed,
		Checksum: "fixture-checksum",
		Blockers: []model.CreditValuationMigrationBlocker{{Code: "non_terminal_preconsume", Count: 2}},
	}

	exitCode := runCreditValuationCommand(
		[]string{"--apply", "--version", "1"},
		&stdout,
		&stderr,
		creditValuationCommandDependencies{
			initMaintenanceDB:  func() (*gorm.DB, error) { return &gorm.DB{}, nil },
			closeMaintenanceDB: func() error { return nil },
			runMigration: func(*gorm.DB, model.CreditValuationMigrationRequest) (model.CreditValuationMigrationReport, error) {
				return expected, model.ErrCreditValuationMigrationBlocked
			},
		},
	)

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	var output creditValuationCommandErrorOutput
	require.NoError(t, common.Unmarshal(stderr.Bytes(), &output))
	require.False(t, output.Success)
	require.Equal(t, CreditValuationCommandCodeMigrationFailed, output.Code)
	require.NotNil(t, output.Report)
	require.Equal(t, expected, *output.Report)
}

func TestRunCreditValuationCommandReportsCloseFailureAfterMigrationFailure(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runCreditValuationCommand(
		[]string{"--apply", "--version", "1"},
		&stdout,
		&stderr,
		creditValuationCommandDependencies{
			initMaintenanceDB:  func() (*gorm.DB, error) { return &gorm.DB{}, nil },
			closeMaintenanceDB: func() error { return errors.New("close failed") },
			runMigration: func(*gorm.DB, model.CreditValuationMigrationRequest) (model.CreditValuationMigrationReport, error) {
				return model.CreditValuationMigrationReport{Version: 1, Status: model.CreditValuationMigrationFailed}, model.ErrCreditValuationMigrationBlocked
			},
		},
	)

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	var output creditValuationCommandErrorOutput
	require.NoError(t, common.Unmarshal(stderr.Bytes(), &output))
	require.Equal(t, CreditValuationCommandCodeDatabaseCloseFailed, output.Code)
	require.Contains(t, output.Message, model.ErrCreditValuationMigrationBlocked.Error())
	require.Contains(t, output.Message, "close failed")
	require.NotNil(t, output.Report)
	require.Equal(t, 1, output.Report.Version)
}

func TestRunCreditValuationCommandDryRunDoesNotCreateSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, dbErr := db.DB(); dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	require.False(t, db.Migrator().HasTable(&model.CreditValuationMigration{}))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runCreditValuationCommand(
		[]string{"--dry-run", "--version", "1"},
		&stdout,
		&stderr,
		creditValuationCommandDependencies{
			initMaintenanceDB:  func() (*gorm.DB, error) { return db, nil },
			closeMaintenanceDB: func() error { return nil },
			runMigration:       model.RunCreditValuationMigration,
		},
	)

	require.Equal(t, 1, exitCode)
	require.Empty(t, stdout.String())
	require.NotEmpty(t, stderr.String())
	require.False(t, db.Migrator().HasTable(&model.CreditValuationMigration{}), "read-only dry-run must not create schema")
}

func TestRunCreditValuationCommandDryRunAndVerifyAreReadOnlyWithExistingSchema(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.UserSubscription{}))
	require.NoError(t, model.MigrateCreditValuationSchema(db))
	require.NoError(t, db.Create(&model.CreditValuationMigration{
		Version: 1, Status: model.CreditValuationMigrationReady, ValuationCurrency: "CNY",
		FxRateNumerator: 1, FxRateDenominator: 1, FxCapturedAt: 1,
	}).Error)
	dryRun, err := model.RunCreditValuationMigration(db, model.CreditValuationMigrationRequest{
		Mode: model.CreditValuationMigrationModeDryRun, Version: 1, BatchSize: 100,
	})
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.CreditValuationMigration{}).Where("version = ?", 1).Update("checksum", dryRun.Checksum).Error)

	var changesBefore int64
	require.NoError(t, db.Raw("SELECT total_changes()").Scan(&changesBefore).Error)
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)
	for _, flag := range []string{"--dry-run", "--verify"} {
		t.Run(flag, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			exitCode := runCreditValuationCommand(
				[]string{flag, "--version", "1"},
				&stdout,
				&stderr,
				creditValuationCommandDependencies{
					initMaintenanceDB:  func() (*gorm.DB, error) { return db, nil },
					closeMaintenanceDB: func() error { return nil },
					runMigration:       model.RunCreditValuationMigration,
				},
			)

			require.Zero(t, exitCode)
			require.Empty(t, stderr.String())
			var output creditValuationCommandSuccessOutput
			require.NoError(t, common.Unmarshal(stdout.Bytes(), &output))
			require.True(t, output.Success)
			require.True(t, output.Report.ReadOnly)
			require.False(t, output.Report.Changed)
			require.Equal(t, dryRun.Checksum, output.Report.Checksum)
			var changesAfter int64
			require.NoError(t, db.Raw("SELECT total_changes()").Scan(&changesAfter).Error)
			require.Equal(t, changesBefore, changesAfter)
		})
	}
}

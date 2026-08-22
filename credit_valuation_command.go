package main

import (
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	CreditValuationCommandDefaultBatchSize = 100

	CreditValuationCommandCodeModeRequired        = "credit_valuation_command_mode_required"
	CreditValuationCommandCodeModeDuplicate       = "credit_valuation_command_mode_duplicate"
	CreditValuationCommandCodeModeConflict        = "credit_valuation_command_mode_conflict"
	CreditValuationCommandCodeVersionRequired     = "credit_valuation_command_version_required"
	CreditValuationCommandCodeVersionInvalid      = "credit_valuation_command_version_invalid"
	CreditValuationCommandCodeBatchSizeInvalid    = "credit_valuation_command_batch_size_invalid"
	CreditValuationCommandCodeBatchSizeNotAllowed = "credit_valuation_command_batch_size_not_allowed"
	CreditValuationCommandCodeReasonRequired      = "credit_valuation_command_reason_required"
	CreditValuationCommandCodeReasonNotAllowed    = "credit_valuation_command_reason_not_allowed"
	CreditValuationCommandCodeFlagDuplicate       = "credit_valuation_command_flag_duplicate"
	CreditValuationCommandCodeFlagValueRequired   = "credit_valuation_command_flag_value_required"
	CreditValuationCommandCodeUnknownFlag         = "credit_valuation_command_unknown_flag"
	CreditValuationCommandCodeUnexpectedArgument  = "credit_valuation_command_unexpected_argument"
	CreditValuationCommandCodeRequestInvalid      = "credit_valuation_command_request_invalid"
	CreditValuationCommandCodeDatabaseOpenFailed  = "credit_valuation_command_database_open_failed"
	CreditValuationCommandCodeMigrationFailed     = "credit_valuation_command_migration_failed"
	CreditValuationCommandCodeDatabaseCloseFailed = "credit_valuation_command_database_close_failed"
	CreditValuationCommandCodeOutputFailed        = "credit_valuation_command_output_failed"
)

type CreditValuationCommandOptions struct {
	Mode      model.CreditValuationMigrationMode
	Version   int
	BatchSize int
	Reason    string
}

type CreditValuationCommandError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *CreditValuationCommandError) Error() string {
	return e.Message
}

func newCreditValuationCommandError(code string, message string) error {
	return &CreditValuationCommandError{Code: code, Message: message}
}

func ParseCreditValuationCommandArgs(args []string) (CreditValuationCommandOptions, error) {
	options := CreditValuationCommandOptions{BatchSize: CreditValuationCommandDefaultBatchSize}
	var modeFlag string
	var versionSet bool
	var batchSizeSet bool
	var reasonSet bool

	for index := 0; index < len(args); index++ {
		argument := args[index]
		switch argument {
		case "--dry-run", "--apply", "--verify", "--repair-missing-as-unknown", "--revalue-historical", "--suspend":
			mode := creditValuationMigrationModeForFlag(argument)
			if modeFlag != "" {
				if modeFlag == argument {
					return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeModeDuplicate, "migration mode flag must not be repeated")
				}
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeModeConflict, "migration mode flags are mutually exclusive")
			}
			modeFlag = argument
			options.Mode = mode
		case "--version":
			if versionSet {
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeFlagDuplicate, "--version must not be repeated")
			}
			value, nextIndex, err := creditValuationCommandFlagValue(args, index, argument)
			if err != nil {
				return CreditValuationCommandOptions{}, err
			}
			version, parseErr := strconv.Atoi(value)
			if parseErr != nil || version <= 0 {
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeVersionInvalid, "--version must be a positive integer")
			}
			versionSet = true
			options.Version = version
			index = nextIndex
		case "--batch-size":
			if batchSizeSet {
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeFlagDuplicate, "--batch-size must not be repeated")
			}
			value, nextIndex, err := creditValuationCommandFlagValue(args, index, argument)
			if err != nil {
				return CreditValuationCommandOptions{}, err
			}
			batchSize, parseErr := strconv.Atoi(value)
			if parseErr != nil || batchSize <= 0 {
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeBatchSizeInvalid, "--batch-size must be a positive integer")
			}
			batchSizeSet = true
			options.BatchSize = batchSize
			index = nextIndex
		case "--reason":
			if reasonSet {
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeFlagDuplicate, "--reason must not be repeated")
			}
			value, nextIndex, err := creditValuationCommandFlagValue(args, index, argument)
			if err != nil {
				return CreditValuationCommandOptions{}, err
			}
			reasonSet = true
			options.Reason = strings.TrimSpace(value)
			index = nextIndex
		default:
			if strings.HasPrefix(argument, "-") {
				return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeUnknownFlag, "unknown credit valuation migration flag")
			}
			return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeUnexpectedArgument, "unexpected positional argument")
		}
	}

	if modeFlag == "" {
		return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeModeRequired, "exactly one migration mode flag is required")
	}
	if !versionSet {
		return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeVersionRequired, "--version is required")
	}
	if batchSizeSet && options.Mode != model.CreditValuationMigrationModeApply && options.Mode != model.CreditValuationMigrationModeRevalueHistorical {
		return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeBatchSizeNotAllowed, "--batch-size is only allowed with --apply or --revalue-historical")
	}
	if options.Mode == model.CreditValuationMigrationModeSuspend {
		if !reasonSet || options.Reason == "" {
			return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeReasonRequired, "--reason is required for --suspend")
		}
	} else if reasonSet {
		return CreditValuationCommandOptions{}, newCreditValuationCommandError(CreditValuationCommandCodeReasonNotAllowed, "--reason is only allowed with --suspend")
	}

	return options, nil
}

func creditValuationMigrationModeForFlag(flag string) model.CreditValuationMigrationMode {
	switch flag {
	case "--dry-run":
		return model.CreditValuationMigrationModeDryRun
	case "--apply":
		return model.CreditValuationMigrationModeApply
	case "--verify":
		return model.CreditValuationMigrationModeVerify
	case "--repair-missing-as-unknown":
		return model.CreditValuationMigrationModeRepairMissingAsUnknown
	case "--revalue-historical":
		return model.CreditValuationMigrationModeRevalueHistorical
	case "--suspend":
		return model.CreditValuationMigrationModeSuspend
	default:
		return ""
	}
}

func creditValuationCommandFlagValue(args []string, index int, flag string) (string, int, error) {
	nextIndex := index + 1
	if nextIndex >= len(args) || strings.HasPrefix(args[nextIndex], "--") {
		return "", index, newCreditValuationCommandError(CreditValuationCommandCodeFlagValueRequired, flag+" requires a value")
	}
	return args[nextIndex], nextIndex, nil
}

type creditValuationCommandDependencies struct {
	initMaintenanceDB  func() (*gorm.DB, error)
	closeMaintenanceDB func() error
	runMigration       func(*gorm.DB, model.CreditValuationMigrationRequest) (model.CreditValuationMigrationReport, error)
}

type creditValuationCommandErrorOutput struct {
	Success bool                                  `json:"success"`
	Code    string                                `json:"code"`
	Message string                                `json:"message"`
	Report  *model.CreditValuationMigrationReport `json:"report,omitempty"`
}

type creditValuationCommandSuccessOutput struct {
	Success bool                                 `json:"success"`
	Report  model.CreditValuationMigrationReport `json:"report"`
}

func RunCreditValuationCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	return runCreditValuationCommand(args, stdout, stderr, creditValuationCommandDependencies{
		initMaintenanceDB:  model.InitMaintenanceDB,
		closeMaintenanceDB: model.CloseMaintenanceDB,
		runMigration:       model.RunCreditValuationMigration,
	})
}

func runCreditValuationCommand(args []string, stdout io.Writer, stderr io.Writer, dependencies creditValuationCommandDependencies) int {
	options, err := ParseCreditValuationCommandArgs(args)
	if err != nil {
		writeCreditValuationCommandError(stderr, err)
		return 2
	}

	request := model.CreditValuationMigrationRequest{
		Mode:      options.Mode,
		Version:   options.Version,
		BatchSize: options.BatchSize,
		Reason:    options.Reason,
	}
	if err := model.ValidateCreditValuationMigrationRequest(request); err != nil {
		writeCreditValuationCommandError(stderr, newCreditValuationCommandError(CreditValuationCommandCodeRequestInvalid, err.Error()))
		return 2
	}

	db, err := dependencies.initMaintenanceDB()
	if err != nil || db == nil {
		message := "maintenance database initialization failed"
		if err != nil {
			message = err.Error()
		}
		writeCreditValuationCommandError(stderr, newCreditValuationCommandError(CreditValuationCommandCodeDatabaseOpenFailed, message))
		return 1
	}

	report, migrationErr := dependencies.runMigration(db, request)
	closeErr := dependencies.closeMaintenanceDB()
	if closeErr != nil {
		combinedErr := closeErr
		if migrationErr != nil {
			combinedErr = errors.Join(migrationErr, closeErr)
		}
		_ = writeCreditValuationCommandJSON(stderr, creditValuationCommandErrorOutput{
			Success: false, Code: CreditValuationCommandCodeDatabaseCloseFailed,
			Message: combinedErr.Error(), Report: &report,
		})
		return 1
	}
	if migrationErr != nil {
		writeCreditValuationCommandMigrationError(stderr, migrationErr, report)
		return 1
	}

	if err := writeCreditValuationCommandJSON(stdout, creditValuationCommandSuccessOutput{Success: true, Report: report}); err != nil {
		writeCreditValuationCommandError(stderr, newCreditValuationCommandError(CreditValuationCommandCodeOutputFailed, err.Error()))
		return 1
	}
	return 0
}

func writeCreditValuationCommandError(writer io.Writer, err error) {
	code := CreditValuationCommandCodeRequestInvalid
	message := err.Error()
	var commandErr *CreditValuationCommandError
	if errors.As(err, &commandErr) {
		code = commandErr.Code
		message = commandErr.Message
	}
	_ = writeCreditValuationCommandJSON(writer, creditValuationCommandErrorOutput{
		Success: false,
		Code:    code,
		Message: message,
	})
}

func writeCreditValuationCommandMigrationError(writer io.Writer, err error, report model.CreditValuationMigrationReport) {
	_ = writeCreditValuationCommandJSON(writer, creditValuationCommandErrorOutput{
		Success: false,
		Code:    CreditValuationCommandCodeMigrationFailed,
		Message: err.Error(),
		Report:  &report,
	})
}

func writeCreditValuationCommandJSON(writer io.Writer, value any) error {
	payload, err := common.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	_, err = writer.Write(payload)
	return err
}

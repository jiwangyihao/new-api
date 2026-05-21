//go:build windows

package resource

import (
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/QuantumNous/new-api/pkg/loadtest/profile"
	"golang.org/x/sys/windows"
)

var procSetProcessAffinityMask = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetProcessAffinityMask")

func ApplyServerLimits(pid int, limits profile.ServerLimits) (ApplyResult, error) {
	result := ApplyResult{
		Status:                  "applied",
		ProcessMemoryLimitBytes: limits.ProcessMemoryLimitBytes,
		CPUAffinityCores:        limits.CPUAffinityCores,
	}
	if pid <= 0 {
		result.Status = "unavailable"
		result.Reason = "pid must be positive"
		return result, fmt.Errorf("pid must be positive")
	}

	access := uint32(windows.PROCESS_SET_INFORMATION | windows.PROCESS_SET_QUOTA | windows.PROCESS_QUERY_LIMITED_INFORMATION)
	process, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		result.Status = "partial"
		result.Reason = "open process failed"
		return result, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)

	var failures []string
	mask := uintptr(0)
	if limits.CPUAffinityCores > 0 {
		mask = affinityMask(limits.CPUAffinityCores)
		result.CPUAffinityMask = mask
		if err := setProcessAffinityMask(process, mask); err != nil {
			failures = append(failures, fmt.Sprintf("set CPU affinity: %v", err))
		} else {
			result.CPUAffinityEnforced = true
		}
	}

	if limits.ProcessMemoryLimitBytes > 0 {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			failures = append(failures, fmt.Sprintf("create job object: %v", err))
		} else {
			defer windows.CloseHandle(job)
			var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
			info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_PROCESS_MEMORY
			if mask != 0 {
				info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_AFFINITY
				info.BasicLimitInformation.Affinity = mask
			}
			info.ProcessMemoryLimit = uintptr(limits.ProcessMemoryLimitBytes)
			if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
				failures = append(failures, fmt.Sprintf("set job object memory limit: %v", err))
			} else if err := windows.AssignProcessToJobObject(job, process); err != nil {
				failures = append(failures, fmt.Sprintf("assign process to job object: %v", err))
			} else {
				result.MemoryLimitEnforced = true
				if mask != 0 {
					result.CPUAffinityEnforced = true
				}
			}
		}
	}

	if len(failures) > 0 {
		result.Status = "partial"
		result.Reason = strings.Join(failures, "; ")
		return result, fmt.Errorf("apply server limits: %s", result.Reason)
	}
	return result, nil
}

func affinityMask(cores int) uintptr {
	if cores <= 0 {
		return 0
	}
	bits := runtime.GOMAXPROCS(0)
	if bits <= 0 {
		bits = 1
	}
	if cores < bits {
		bits = cores
	}
	if uintptr(bits) >= unsafe.Sizeof(uintptr(0))*8 {
		return ^uintptr(0)
	}
	return (uintptr(1) << bits) - 1
}

func setProcessAffinityMask(process windows.Handle, mask uintptr) error {
	if mask == 0 {
		return fmt.Errorf("affinity mask is zero")
	}
	ret, _, callErr := procSetProcessAffinityMask.Call(uintptr(process), mask)
	if ret == 0 {
		if callErr != windows.ERROR_SUCCESS {
			return callErr
		}
		return fmt.Errorf("SetProcessAffinityMask failed")
	}
	return nil
}

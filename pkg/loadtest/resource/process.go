package resource

import (
	"strings"
	"syscall"

	artifactnet "github.com/shirou/gopsutil/net"
	"github.com/shirou/gopsutil/process"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
)

func SampleProcess(pid int) artifact.ProcessSnapshot {
	snapshot := artifact.ProcessSnapshot{PID: pid}
	if pid <= 0 {
		snapshot.Statused = artifact.Statused{Status: "unavailable", Reason: "pid must be positive"}
		return snapshot
	}

	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		snapshot.Statused = artifact.Statused{Status: "unavailable", Reason: "process unavailable"}
		return snapshot
	}

	var failures []string
	if mem, err := proc.MemoryInfo(); err == nil && mem != nil {
		snapshot.RSSBytes = mem.RSS
	} else if err != nil {
		failures = append(failures, "memory")
	}
	if cpuPercent, err := proc.CPUPercent(); err == nil {
		snapshot.CPUPercent = cpuPercent
	} else {
		failures = append(failures, "cpu_percent")
	}
	if threads, err := proc.NumThreads(); err == nil && threads > 0 {
		snapshot.ThreadCount = int(threads)
	} else if err != nil {
		failures = append(failures, "threads")
	}
	if fds, err := proc.NumFDs(); err == nil && fds > 0 {
		snapshot.HandleCount = int(fds)
	} else if files, err := proc.OpenFiles(); err == nil && len(files) > 0 {
		snapshot.HandleCount = len(files)
	}
	if times, err := proc.Times(); err == nil && times != nil {
		snapshot.CPUTimeSeconds = times.User + times.System
	} else if err != nil {
		failures = append(failures, "cpu_time")
	}
	if sockets, err := artifactnet.ConnectionsPid("tcp", int32(pid)); err == nil {
		snapshot.OpenTCPSockets = len(sockets)
	} else if conns, connErr := proc.Connections(); connErr == nil {
		snapshot.OpenTCPSockets = countTCPConnections(conns)
	}

	if snapshot.RSSBytes == 0 && snapshot.ThreadCount == 0 && snapshot.CPUTimeSeconds == 0 && len(failures) > 0 {
		snapshot.Statused = artifact.Statused{Status: "unavailable", Reason: "process metrics unavailable"}
		return snapshot
	}
	if len(failures) > 0 {
		snapshot.Statused = artifact.Statused{Status: "ok", Reason: "partial metrics unavailable: " + strings.Join(failures, ",")}
		return snapshot
	}
	snapshot.Statused = artifact.Statused{Status: "ok"}
	return snapshot
}

func countTCPConnections(conns []artifactnet.ConnectionStat) int {
	count := 0
	for _, conn := range conns {
		if conn.Type == syscall.SOCK_STREAM {
			count++
		}
	}
	return count
}

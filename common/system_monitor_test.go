package common

import (
	"testing"
)

func TestRefreshSystemStatusPopulatesMemoryAndDiskWhenMonitorDisabled(t *testing.T) {
	latestSystemStatus.Store(SystemStatus{})
	t.Cleanup(func() { latestSystemStatus.Store(SystemStatus{}) })

	status := RefreshSystemStatus()

	if status.MemoryUsage <= 0 {
		t.Fatalf("expected memory usage to be populated, got %.2f", status.MemoryUsage)
	}
	if status.DiskUsage <= 0 {
		t.Fatalf("expected disk usage to be populated, got %.2f", status.DiskUsage)
	}
	cached := GetSystemStatus()
	if cached.MemoryUsage != status.MemoryUsage || cached.DiskUsage != status.DiskUsage {
		t.Fatalf("expected refreshed status to be cached, got cached=%+v refreshed=%+v", cached, status)
	}
}

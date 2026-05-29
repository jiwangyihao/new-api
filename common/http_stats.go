package common

import "sync/atomic"

type HTTPStatsInfo struct {
	ActiveConnections int64 `json:"active_connections"`
}

var activeHTTPConnections atomic.Int64

func IncrementActiveConnections() {
	activeHTTPConnections.Add(1)
}

func DecrementActiveConnections() {
	activeHTTPConnections.Add(-1)
}

func GetHTTPStats() HTTPStatsInfo {
	return HTTPStatsInfo{ActiveConnections: activeHTTPConnections.Load()}
}

func GetActiveConnections() int64 {
	return activeHTTPConnections.Load()
}

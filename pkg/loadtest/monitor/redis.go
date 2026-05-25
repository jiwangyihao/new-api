package monitor

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	"github.com/QuantumNous/new-api/pkg/loadtest/localguard"
)

func ParseRedisInfo(info string) artifact.RedisSnapshot {
	snapshot := artifact.RedisSnapshot{Statused: artifact.Statused{Status: "ok"}, Info: make(map[string]string), Keyspace: make(map[string]int64)}
	for _, rawLine := range strings.Split(info, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		snapshot.Info[key] = value
		switch key {
		case "connected_clients":
			if parsed, err := strconv.Atoi(value); err == nil {
				snapshot.ConnectedClients = parsed
			}
		case "used_memory":
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
				snapshot.UsedMemoryBytes = parsed
			}
		case "used_memory_rss":
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
				snapshot.UsedMemoryRSSBytes = parsed
			}
		case "mem_fragmentation_ratio":
			if parsed, err := strconv.ParseFloat(value, 64); err == nil {
				snapshot.MemFragmentationRatio = parsed
			}
		case "instantaneous_ops_per_sec":
			if parsed, err := strconv.Atoi(value); err == nil {
				snapshot.InstantaneousOpsPerSec = parsed
			}
		case "total_commands_processed":
			if parsed, err := strconv.ParseUint(value, 10, 64); err == nil {
				snapshot.TotalCommandsProcessed = parsed
			}
		default:
			if strings.HasPrefix(key, "db") {
				if keys, ok := parseRedisKeyspaceKeys(value); ok {
					snapshot.Keyspace[key] = keys
				}
			}
		}
	}
	if len(snapshot.Info) == 0 {
		snapshot.Info = nil
	}
	if len(snapshot.Keyspace) == 0 {
		snapshot.Keyspace = nil
	}
	return snapshot
}

func LoadRedisSnapshot(ctx context.Context, addr string) artifact.RedisSnapshot {
	if err := localguard.ValidateRedisAddr(addr); err != nil {
		return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "config: " + err.Error()}}
	}
	client, err := newRedisClient(addr)
	if err != nil {
		return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "redis client: " + err.Error()}}
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	info, err := client.Info(ctx).Result()
	if err != nil {
		return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: "redis INFO: " + err.Error()}}
	}
	snapshot := ParseRedisInfo(info)
	if snapshot.Status == "" {
		snapshot.Status = "ok"
	}
	return snapshot
}

func newRedisClient(addr string) (*redis.Client, error) {
	if strings.Contains(addr, "://") {
		options, err := redis.ParseURL(addr)
		if err != nil {
			return nil, err
		}
		options.DialTimeout = 500 * time.Millisecond
		options.ReadTimeout = 500 * time.Millisecond
		options.WriteTimeout = 500 * time.Millisecond
		options.PoolSize = 1
		options.MinIdleConns = 0
		return redis.NewClient(options), nil
	}
	return redis.NewClient(&redis.Options{Addr: addr, PoolSize: 1, MinIdleConns: 0, DialTimeout: 500 * time.Millisecond, ReadTimeout: 500 * time.Millisecond, WriteTimeout: 500 * time.Millisecond}), nil
}

func parseRedisKeyspaceKeys(value string) (int64, bool) {
	for _, part := range strings.Split(value, ",") {
		name, raw, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(name) != "keys" {
			continue
		}
		parsed, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	}
	return 0, false
}

func redisUnavailable(reason string, args ...any) artifact.RedisSnapshot {
	return artifact.RedisSnapshot{Statused: artifact.Statused{Status: "unavailable", Reason: fmt.Sprintf(reason, args...)}}
}

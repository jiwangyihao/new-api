package resource

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/pkg/loadtest/artifact"
	loadtestconfig "github.com/QuantumNous/new-api/pkg/loadtest/config"
)

func CheckPortsClosed(rc artifact.RunContext, ports []int) artifact.PortsClosedArtifact {
	result := artifact.PortsClosedArtifact{
		SchemaVersion: artifact.SchemaVersion,
		RunContext:    rc,
		Ports:         make(map[string]string, len(ports)),
		Passed:        true,
	}
	for _, port := range ports {
		key := strconv.Itoa(port)
		if port <= 0 || port > 65535 {
			result.Ports[key] = "invalid"
			result.Passed = false
			continue
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", key), 300*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			result.Ports[key] = "open"
			result.Passed = false
			continue
		}
		if isTimeout(err) {
			result.Ports[key] = "timeout"
			result.Passed = false
			continue
		}
		result.Ports[key] = "closed"
	}
	return result
}

func PortsFromConfig(file loadtestconfig.File) ([]int, error) {
	ports := make([]int, 0, 5)
	seen := make(map[int]struct{}, 5)
	add := func(name string, port int, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if port <= 0 || port > 65535 {
			return fmt.Errorf("%s: port is invalid", name)
		}
		if _, ok := seen[port]; !ok {
			seen[port] = struct{}{}
			ports = append(ports, port)
		}
		return nil
	}
	postgresPort, err := portFromURL(file.Postgres.DSN, "PostgreSQL")
	if err := addInfraPort("postgres.dsn", postgresPort, err, 5432); err != nil {
		return nil, err
	}
	if err := add("postgres.dsn", postgresPort, nil); err != nil {
		return nil, err
	}
	redisPort, err := portFromRedisAddr(file.Redis.Addr)
	if err := addInfraPort("redis.addr", redisPort, err, 6379); err != nil {
		return nil, err
	}
	if err := add("redis.addr", redisPort, nil); err != nil {
		return nil, err
	}
	serverPort, err := portFromHostPort(file.Server.Host, strconv.Itoa(file.Server.Port))
	if err := add("server", serverPort, err); err != nil {
		return nil, err
	}
	pprofPort, err := portFromListenAddr(file.Server.PprofAddr)
	if err := add("server.pprof_addr", pprofPort, err); err != nil {
		return nil, err
	}
	mockPort, err := portFromURL(file.MockUpstream.BaseURL, "mock upstream")
	if err := add("mock_upstream.base_url", mockPort, err); err != nil {
		return nil, err
	}
	return ports, nil
}

func addInfraPort(name string, port int, err error, defaultPort int) error {
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if port == defaultPort {
		return fmt.Errorf("%s: default infrastructure port is not allowed", name)
	}
	return nil
}

func portFromURL(raw string, kind string) (int, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid URL")
	}
	if u.Host == "" || u.Hostname() == "" {
		return 0, fmt.Errorf("%s host is required", kind)
	}
	if err := requireLoopbackHost(u.Hostname()); err != nil {
		return 0, err
	}
	port := u.Port()
	if port == "" {
		return 0, fmt.Errorf("port is required")
	}
	return parsePort(port)
}

func portFromRedisAddr(addr string) (int, error) {
	addr = strings.TrimSpace(addr)
	if strings.Contains(addr, "://") {
		return portFromURL(addr, "Redis")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, fmt.Errorf("invalid Redis address")
	}
	return portFromHostPort(host, port)
}

func portFromListenAddr(addr string) (int, error) {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return 0, fmt.Errorf("invalid listen address")
	}
	return portFromHostPort(host, port)
}

func portFromHostPort(host string, port string) (int, error) {
	if err := requireLoopbackHost(host); err != nil {
		return 0, err
	}
	return parsePort(port)
}

func parsePort(port string) (int, error) {
	if strings.TrimSpace(port) == "" {
		return 0, fmt.Errorf("port is required")
	}
	parsed, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("port is invalid")
	}
	if parsed <= 0 || parsed > 65535 {
		return 0, fmt.Errorf("port is invalid")
	}
	return parsed, nil
}

func requireLoopbackHost(host string) error {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("host must be loopback")
	}
	return nil
}

func isTimeout(err error) bool {
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return true
	}
	return false
}

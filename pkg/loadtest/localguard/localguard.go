package localguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var allowedAPIKeys = map[string]struct{}{
	"sk-loadtestsub":     {},
	"sk-loadtestcompat":  {},
	"sk-loadtestinvalid": {},
}

var providerCredentialEnvNames = map[string]struct{}{
	"OPENAI_API_KEY":       {},
	"ANTHROPIC_API_KEY":    {},
	"AZURE_OPENAI_API_KEY": {},
	"GEMINI_API_KEY":       {},
	"OPENROUTER_API_KEY":   {},
	"COHERE_API_KEY":       {},
	"MISTRAL_API_KEY":      {},
	"DASHSCOPE_API_KEY":    {},
	"VOLCENGINE_API_KEY":   {},
}

var providerEndpointEnvNames = map[string]struct{}{
	"OPENAI_BASE_URL":       {},
	"OPENAI_API_BASE":       {},
	"ANTHROPIC_BASE_URL":    {},
	"AZURE_OPENAI_ENDPOINT": {},
	"UPSTREAM_BASE_URL":     {},
	"GEMINI_BASE_URL":       {},
	"OPENROUTER_BASE_URL":   {},
}

// ValidateURL accepts only HTTP URLs whose host is loopback, localhost, or resolves only to loopback addresses.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" {
		return fmt.Errorf("URL scheme must be http")
	}
	if u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("URL host is required")
	}
	if err := validateLoopbackHost(u.Hostname()); err != nil {
		return fmt.Errorf("unsafe URL host: %w", err)
	}
	return nil
}

// ValidatePostgresDSN accepts only postgres:// or postgresql:// URLs pointing at a loopback host and a loadtest database/user on a non-default port.
func ValidatePostgresDSN(dsn string) error {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return fmt.Errorf("invalid PostgreSQL DSN")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("PostgreSQL DSN must use postgres:// or postgresql://")
	}
	if u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("PostgreSQL DSN host is required")
	}
	if err := validateLoopbackHost(u.Hostname()); err != nil {
		return fmt.Errorf("unsafe PostgreSQL host: %w", err)
	}
	for _, forbidden := range []string{"host", "hostaddr", "port", "dbname", "database", "service"} {
		if u.Query().Has(forbidden) {
			return fmt.Errorf("PostgreSQL DSN query parameter %q is not allowed", forbidden)
		}
	}
	if port := u.Port(); port == "" || port == "5432" {
		return fmt.Errorf("PostgreSQL default port is not allowed")
	}
	if !strings.Contains(strings.ToLower(u.User.Username()), "loadtest") {
		return fmt.Errorf("PostgreSQL user must contain loadtest")
	}
	database := strings.TrimPrefix(u.EscapedPath(), "/")
	if database == "" {
		return fmt.Errorf("PostgreSQL database is required")
	}
	unescapedDatabase, err := url.PathUnescape(database)
	if err != nil {
		return fmt.Errorf("invalid PostgreSQL database name")
	}
	if !strings.Contains(strings.ToLower(unescapedDatabase), "loadtest") {
		return fmt.Errorf("PostgreSQL database must contain loadtest")
	}
	return nil
}

// ValidateRedisAddr accepts redis:// URLs and host:port addresses whose host is loopback and port is not the default Redis port.
func ValidateRedisAddr(addr string) error {
	host, port, err := redisHostPort(strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	if port == "" || port == "6379" {
		return fmt.Errorf("Redis default port is not allowed")
	}
	if err := validateLoopbackHost(host); err != nil {
		return fmt.Errorf("unsafe Redis host: %w", err)
	}
	return nil
}

// ValidateLoadtestAPIKey accepts only fixed loadtest API keys.
func ValidateLoadtestAPIKey(key string) error {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "Bearer ")
	if _, ok := allowedAPIKeys[key]; !ok {
		return fmt.Errorf("API key is not an allowed loadtest key")
	}
	return nil
}

// ValidateAPIKey accepts only fixed loadtest API keys.
func ValidateAPIKey(key string) error {
	return ValidateLoadtestAPIKey(key)
}

// RejectDefaultInfraPorts rejects isolated infra settings that point at default PostgreSQL or Redis ports.
func RejectDefaultInfraPorts(postgresDSN, redisAddr string) error {
	if strings.TrimSpace(postgresDSN) != "" {
		u, err := url.Parse(strings.TrimSpace(postgresDSN))
		if err != nil {
			return fmt.Errorf("invalid PostgreSQL DSN")
		}
		if u.Port() == "" || u.Port() == "5432" {
			return fmt.Errorf("PostgreSQL default port is not allowed")
		}
	}
	if strings.TrimSpace(redisAddr) != "" {
		_, port, err := redisHostPort(strings.TrimSpace(redisAddr))
		if err != nil {
			return err
		}
		if port == "" || port == "6379" {
			return fmt.Errorf("Redis default port is not allowed")
		}
	}
	return nil
}

// ValidateCleanEnv rejects inherited production credentials and non-isolated loadtest infrastructure env.
func ValidateCleanEnv(env map[string]string) error {
	for key, value := range env {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if isProviderCredentialEnv(key) {
			return fmt.Errorf("provider credential env %s is not allowed", key)
		}
		if isProviderEndpointEnv(key) {
			return fmt.Errorf("provider upstream env %s is not allowed", key)
		}
	}
	if dsn := strings.TrimSpace(env["SQL_DSN"]); dsn != "" {
		if err := ValidatePostgresDSN(dsn); err != nil {
			return fmt.Errorf("SQL_DSN is not a loadtest PostgreSQL DSN")
		}
	} else if _, ok := env["SQL_DSN"]; ok {
		return fmt.Errorf("SQL_DSN is required")
	}
	if dsn := strings.TrimSpace(env["LOG_SQL_DSN"]); dsn != "" {
		if err := ValidatePostgresDSN(dsn); err != nil {
			return fmt.Errorf("LOG_SQL_DSN is not a loadtest PostgreSQL DSN")
		}
	}
	if redis := strings.TrimSpace(env["REDIS_CONN_STRING"]); redis != "" {
		if err := ValidateRedisAddr(redis); err != nil {
			return fmt.Errorf("REDIS_CONN_STRING is not a loadtest Redis address")
		}
	} else if _, ok := env["REDIS_CONN_STRING"]; ok {
		return fmt.Errorf("REDIS_CONN_STRING is required")
	}
	return nil
}

func ValidateCleanWorkDir(workDir string) error {
	if strings.TrimSpace(workDir) == "" {
		return fmt.Errorf("work-dir is required")
	}
	if _, err := os.Stat(filepath.Join(workDir, ".env")); err == nil {
		return fmt.Errorf("work-dir .env is not allowed")
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ValidateListenAddr accepts host:port listen addresses whose host is loopback.
func ValidateListenAddr(addr string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("invalid listen address")
	}
	if host == "" {
		return fmt.Errorf("listen host is required")
	}
	if port == "" {
		return fmt.Errorf("listen port is required")
	}
	if err := validateLoopbackHost(host); err != nil {
		return fmt.Errorf("unsafe listen host: %w", err)
	}
	return nil
}

// ValidateAny dispatches a value to the narrowest loadtest safety validator and rejects unknown values.
func ValidateAny(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("value is required")
	}
	if strings.HasPrefix(value, "Bearer ") || strings.HasPrefix(value, "sk-") {
		return ValidateLoadtestAPIKey(value)
	}
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid URL-like value")
		}
		switch u.Scheme {
		case "http", "https":
			return ValidateURL(value)
		case "postgres", "postgresql":
			return ValidatePostgresDSN(value)
		case "redis":
			return ValidateRedisAddr(value)
		default:
			return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
		}
	}
	if host, port, err := net.SplitHostPort(value); err == nil {
		if isDefaultInfraPort(port) {
			return fmt.Errorf("default PostgreSQL/Redis ports are not allowed")
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("host is required")
		}
		return ValidateListenAddr(value)
	}
	return fmt.Errorf("unsupported value")
}

func redisHostPort(addr string) (string, string, error) {
	if addr == "" {
		return "", "", fmt.Errorf("Redis address is required")
	}
	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err != nil {
			return "", "", fmt.Errorf("invalid Redis URL")
		}
		if u.Scheme != "redis" {
			return "", "", fmt.Errorf("Redis address must use redis://")
		}
		if u.Host == "" || u.Hostname() == "" {
			return "", "", fmt.Errorf("Redis host is required")
		}
		return u.Hostname(), u.Port(), nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", fmt.Errorf("invalid Redis address")
	}
	if host == "" || port == "" {
		return "", "", fmt.Errorf("Redis host and port are required")
	}
	return host, port, nil
}

func isProviderCredentialEnv(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	if _, ok := providerCredentialEnvNames[key]; ok {
		return true
	}
	return strings.HasSuffix(key, "_API_KEY") || key == "API_KEY"
}

func isProviderEndpointEnv(key string) bool {
	key = strings.ToUpper(strings.TrimSpace(key))
	if _, ok := providerEndpointEnvNames[key]; ok {
		return true
	}
	return strings.HasSuffix(key, "_BASE_URL") || strings.HasSuffix(key, "_ENDPOINT")
}

func isDefaultInfraPort(port string) bool {
	return strings.TrimSpace(port) == "5432" || strings.TrimSpace(port) == "6379"
}

func validateLoopbackHost(host string) error {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("host is not loopback")
	}
	ips, err := lookupHostIPs(host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("host resolved to no addresses")
	}
	for _, ip := range ips {
		if !ip.IsLoopback() {
			return fmt.Errorf("resolved address is not loopback")
		}
	}
	return nil
}

func lookupHostIPs(host string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("host lookup timed out")
		}
		return nil, fmt.Errorf("host lookup failed")
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

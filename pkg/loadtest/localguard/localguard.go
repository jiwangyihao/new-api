package localguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

var allowedAPIKeys = map[string]struct{}{
	"sk-loadtestsub":     {},
	"sk-loadtestcompat":  {},
	"sk-loadtestinvalid": {},
}

// ValidateURL accepts only HTTP URLs whose host is loopback, localhost, or resolves only to loopback addresses.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("URL scheme must be http")
	}
	if u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("URL host is required")
	}
	if err := validateLoopbackHost(u.Hostname()); err != nil {
		return fmt.Errorf("unsafe URL host %q: %w", u.Hostname(), err)
	}
	return nil
}

// ValidatePostgresDSN accepts only postgres:// or postgresql:// URLs pointing at a loopback host and a loadtest database.
func ValidatePostgresDSN(dsn string) error {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return fmt.Errorf("invalid PostgreSQL DSN: %w", err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return fmt.Errorf("PostgreSQL DSN must use postgres:// or postgresql://")
	}
	if u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("PostgreSQL DSN host is required")
	}
	if err := validateLoopbackHost(u.Hostname()); err != nil {
		return fmt.Errorf("unsafe PostgreSQL host %q: %w", u.Hostname(), err)
	}
	for _, forbidden := range []string{"host", "hostaddr", "port", "dbname", "database", "service"} {
		if u.Query().Has(forbidden) {
			return fmt.Errorf("PostgreSQL DSN query parameter %q is not allowed", forbidden)
		}
	}
	database := strings.TrimPrefix(u.EscapedPath(), "/")
	if database == "" {
		return fmt.Errorf("PostgreSQL database is required")
	}
	unescapedDatabase, err := url.PathUnescape(database)
	if err != nil {
		return fmt.Errorf("invalid PostgreSQL database name: %w", err)
	}
	if !strings.Contains(strings.ToLower(unescapedDatabase), "loadtest") {
		return fmt.Errorf("PostgreSQL database %q must contain loadtest", unescapedDatabase)
	}
	return nil
}

// ValidateRedisAddr accepts redis:// URLs and host:port addresses whose host is loopback.
func ValidateRedisAddr(addr string) error {
	host, err := redisHost(strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	if err := validateLoopbackHost(host); err != nil {
		return fmt.Errorf("unsafe Redis host %q: %w", host, err)
	}
	return nil
}

// ValidateAPIKey accepts only fixed loadtest API keys.
func ValidateAPIKey(key string) error {
	key = strings.TrimSpace(key)
	key = strings.TrimPrefix(key, "Bearer ")
	if _, ok := allowedAPIKeys[key]; !ok {
		return fmt.Errorf("API key is not an allowed loadtest key")
	}
	return nil
}

// ValidateListenAddr accepts host:port listen addresses whose host is loopback.
func ValidateListenAddr(addr string) error {
	host, port, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if host == "" {
		return fmt.Errorf("listen host is required")
	}
	if port == "" {
		return fmt.Errorf("listen port is required")
	}
	if err := validateLoopbackHost(host); err != nil {
		return fmt.Errorf("unsafe listen host %q: %w", host, err)
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
		return ValidateAPIKey(value)
	}
	if strings.Contains(value, "://") {
		u, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("invalid URL-like value: %w", err)
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
	if _, _, err := net.SplitHostPort(value); err == nil {
		return ValidateListenAddr(value)
	}
	return fmt.Errorf("unsupported value")
}

func redisHost(addr string) (string, error) {
	if addr == "" {
		return "", fmt.Errorf("Redis address is required")
	}
	if strings.Contains(addr, "://") {
		u, err := url.Parse(addr)
		if err != nil {
			return "", fmt.Errorf("invalid Redis URL: %w", err)
		}
		if u.Scheme != "redis" {
			return "", fmt.Errorf("Redis address must use redis://")
		}
		if u.Host == "" || u.Hostname() == "" {
			return "", fmt.Errorf("Redis host is required")
		}
		return u.Hostname(), nil
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid Redis address: %w", err)
	}
	if host == "" || port == "" {
		return "", fmt.Errorf("Redis host and port are required")
	}
	return host, nil
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
			return fmt.Errorf("resolved address %s is not loopback", ip.String())
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
		return nil, fmt.Errorf("host lookup failed: %w", err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		ips = append(ips, addr.IP)
	}
	return ips, nil
}

package common

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
)

func TestApplyRedisPoolOptionsUsesIdleTimeoutEnv(t *testing.T) {
	t.Setenv("REDIS_IDLE_TIMEOUT_SECONDS", "3")
	opt := &redis.Options{PoolSize: 512}
	applyRedisPoolOptions(opt)
	if opt.PoolSize != 256 {
		t.Fatalf("PoolSize = %d, want capped 256", opt.PoolSize)
	}
	if opt.IdleTimeout != 3*time.Second {
		t.Fatalf("IdleTimeout = %s, want 3s", opt.IdleTimeout)
	}
	if opt.MinIdleConns != 0 {
		t.Fatalf("MinIdleConns = %d, want 0", opt.MinIdleConns)
	}
	if opt.IdleCheckFrequency != time.Second {
		t.Fatalf("IdleCheckFrequency = %s, want 1s", opt.IdleCheckFrequency)
	}
}

func TestApplyRedisPoolOptionsPreventsIdlePoolFromSaturatingManagedRedis(t *testing.T) {
	t.Setenv("REDIS_IDLE_TIMEOUT_SECONDS", "1")
	opt := &redis.Options{PoolSize: 512, MinIdleConns: 512}
	applyRedisPoolOptions(opt)
	if opt.MinIdleConns != 0 {
		t.Fatalf("MinIdleConns = %d, want 0 so loadtest does not keep hundreds of idle Redis sockets", opt.MinIdleConns)
	}
}

func TestApplyRedisPoolOptionsCapsPoolSizeWhenIdleTimeoutEnabled(t *testing.T) {
	t.Setenv("REDIS_IDLE_TIMEOUT_SECONDS", "1")
	opt := &redis.Options{PoolSize: 512}
	applyRedisPoolOptions(opt)
	if opt.PoolSize > 256 {
		t.Fatalf("PoolSize = %d, want <= 256 with idle timeout enabled", opt.PoolSize)
	}
}

func TestApplyRedisPoolOptionsKeepsDefaultWithoutEnv(t *testing.T) {
	t.Setenv("REDIS_IDLE_TIMEOUT_SECONDS", "")
	opt := &redis.Options{PoolSize: 512}
	applyRedisPoolOptions(opt)
	if opt.IdleTimeout != 0 {
		t.Fatalf("IdleTimeout = %s, want redis default", opt.IdleTimeout)
	}
}

package common

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func setupRedisHashTest(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	oldRedisEnabled := RedisEnabled
	oldRDB := RDB
	server, err := miniredis.Run()
	require.NoError(t, err)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	RedisEnabled = true
	RDB = client
	t.Cleanup(func() {
		_ = client.Close()
		RedisEnabled = oldRedisEnabled
		RDB = oldRDB
		server.Close()
	})
	return server
}

func TestRedisHSetFieldUpdatesExistingHashWithoutTTL(t *testing.T) {
	setupRedisHashTest(t)
	ctx := context.Background()
	require.NoError(t, RDB.HSet(ctx, "user:redis-hset-no-ttl", "Id", "1").Err())
	require.NoError(t, RedisHSetField("user:redis-hset-no-ttl", "Setting", `{"codex_pro_mode":"off"}`))

	fields, err := RDB.HGetAll(ctx, "user:redis-hset-no-ttl").Result()
	require.NoError(t, err)
	require.Equal(t, "1", fields["Id"])
	require.Equal(t, `{"codex_pro_mode":"off"}`, fields["Setting"])
	require.Equal(t, time.Duration(-1), RDB.TTL(ctx, "user:redis-hset-no-ttl").Val())
}

func TestRedisHSetFieldPreservesExistingTTL(t *testing.T) {
	server := setupRedisHashTest(t)
	ctx := context.Background()
	require.NoError(t, RDB.HSet(ctx, "user:redis-hset-ttl", "Id", "1").Err())
	require.NoError(t, RDB.Expire(ctx, "user:redis-hset-ttl", 30*time.Second).Err())

	require.NoError(t, RedisHSetField("user:redis-hset-ttl", "Setting", `{"codex_pro_mode":"off"}`))

	fields, err := RDB.HGetAll(ctx, "user:redis-hset-ttl").Result()
	require.NoError(t, err)
	require.Equal(t, "1", fields["Id"])
	require.Equal(t, `{"codex_pro_mode":"off"}`, fields["Setting"])
	ttl := server.TTL("user:redis-hset-ttl")
	require.Greater(t, ttl, time.Duration(0))
	require.LessOrEqual(t, ttl, 30*time.Second)
}

func TestRedisHSetFieldDoesNotCreateMissingHash(t *testing.T) {
	setupRedisHashTest(t)
	ctx := context.Background()
	require.NoError(t, RedisHSetField("user:redis-hset-missing", "Setting", `{"codex_pro_mode":"off"}`))

	exists, err := RDB.Exists(ctx, "user:redis-hset-missing").Result()
	require.NoError(t, err)
	require.Equal(t, int64(0), exists)
}

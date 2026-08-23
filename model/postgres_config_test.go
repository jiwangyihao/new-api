package model

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestNewPostgreSQLConnConfigDisablesUnusedPGXCaches(t *testing.T) {
	const dsn = "postgres://new_api:secret@localhost:5432/new_api?sslmode=disable&TimeZone=Asia%2FShanghai"

	defaultConfig, err := pgx.ParseConfig(dsn)
	require.NoError(t, err)
	require.Positive(t, defaultConfig.StatementCacheCapacity)
	require.Positive(t, defaultConfig.DescriptionCacheCapacity)

	config, err := newPostgreSQLConnConfig(dsn)
	require.NoError(t, err)
	require.Equal(t, pgx.QueryExecModeSimpleProtocol, config.DefaultQueryExecMode)
	require.Zero(t, config.StatementCacheCapacity)
	require.Zero(t, config.DescriptionCacheCapacity)
	require.Equal(t, defaultConfig.ConnString(), config.ConnString(), "disabling unused client caches must preserve the connection string")
	require.Equal(t, defaultConfig.RuntimeParams, config.RuntimeParams, "disabling unused client caches must preserve runtime parameters")
}

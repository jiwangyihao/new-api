package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCommandUsesCleanAllowlistEnvironment(t *testing.T) {
	hostile := map[string]string{"SQL_DSN": "postgresql://prod:prod@example.com:5432/prod", "LOG_SQL_DSN": "postgresql://prod:prod@example.com:5432/prod", "REDIS_CONN_STRING": "redis://example.com:6379/0", "OPENAI_API_KEY": "sk-real-production", "CHANNEL_TEST_FREQUENCY": "1", "CHANNEL_UPDATE_FREQUENCY": "1", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": "true", "PYROSCOPE_URL": "http://example.com/pyroscope", "SYNC_UPSTREAM_BASE": "https://basellm.github.io/llm-metadata"}
	for k, v := range hostile {
		t.Setenv(k, v)
	}
	dir := t.TempDir()
	binary := filepath.Join(dir, "new-api")
	if err := os.WriteFile(binary, []byte("placeholder"), 0o700); err != nil {
		t.Fatal(err)
	}
	workDir := filepath.Join(dir, "runtime")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	env := safeEnv()
	cmd, err := BuildCommand(Config{Binary: binary, WorkDir: workDir, Env: env, PIDFile: filepath.Join(dir, "new-api.pid"), StdoutLog: filepath.Join(dir, "stdout.log"), StderrLog: filepath.Join(dir, "stderr.log")})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Dir != workDir {
		t.Fatalf("cmd.Dir = %q", cmd.Dir)
	}
	if len(cmd.Env) == 0 {
		t.Fatal("empty Env would inherit parent")
	}
	joined := strings.Join(cmd.Env, "\n")
	for _, forbidden := range []string{"OPENAI_API_KEY=", "example.com", "CHANNEL_UPDATE_FREQUENCY=1", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=true", "PYROSCOPE_URL=http"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("leaked hostile env %q in %s", forbidden, joined)
		}
	}
	for _, required := range []string{"CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED=false", "CHANNEL_UPDATE_FREQUENCY=0", "UPDATE_TASK=false", "CHANNEL_TEST_FREQUENCY=0", "PYROSCOPE_URL=", "SYNC_UPSTREAM_BASE=", "RetryTimes=0", "AutomaticRetryStatusCodes="} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing safe env %q in %s", required, joined)
		}
	}
}

func TestBuildCommandRejectsUnsafeEnvAndDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OPENAI_API_KEY=sk-real"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOST": "0.0.0.0", "SQL_DSN": "postgresql://prod:prod@example.com:5432/prod", "REDIS_CONN_STRING": "redis://example.com:6379/0"}
	_, err := BuildCommand(Config{Binary: filepath.Join(dir, "new-api"), WorkDir: dir, Env: env, PIDFile: filepath.Join(dir, "new-api.pid")})
	if err == nil {
		t.Fatal("unsafe env/workdir accepted")
	}
}

func safeEnv() map[string]string {
	return map[string]string{"HOST": "127.0.0.1", "PORT": "13080", "PPROF_ADDR": "127.0.0.1:8005", "SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "LOG_SQL_DSN": "", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0", "ENABLE_PPROF": "true", "LOADTEST_RUNTIME_STATS_ENABLED": "true", "LOADTEST_PROFILE_BLOCK_RATE": "1000", "LOADTEST_PROFILE_MUTEX_FRACTION": "5", "GOMAXPROCS": "2", "GOGC": "100", "BATCH_UPDATE_ENABLED": "true", "SQL_MAX_OPEN_CONNS": "10", "SQL_MAX_IDLE_CONNS": "5", "SQL_MAX_LIFETIME": "60", "CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED": "false", "CHANNEL_UPDATE_FREQUENCY": "0", "UPDATE_TASK": "false", "CHANNEL_TEST_FREQUENCY": "0", "PYROSCOPE_URL": "", "SYNC_UPSTREAM_BASE": "", "RetryTimes": "0", "AutomaticRetryStatusCodes": ""}
}

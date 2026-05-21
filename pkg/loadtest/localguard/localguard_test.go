package localguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalGuardRejectsUnsafeTargets(t *testing.T) {
	cases := []string{
		"https://api.openai.com/v1",
		"postgresql://new_api:secret@example.com:5432/new_api",
		"redis://10.0.0.2:6379/0",
		"sk-loadtest-subscription",
		"",
		"http://192.168.1.10:19080",
		"http://service.internal:19080",
		"postgresql://new_api:secret@10.0.0.2:5432/new_api_loadtest",
		"postgresql://new_api:secret@127.0.0.1:5432/new_api",
		"redis://example.com:6379/0",
		"postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?host=example.com",
		"postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?hostaddr=10.0.0.2",
		"postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?dbname=new_api_loadtest",
	}
	for _, value := range cases {
		if err := ValidateAny(value); err == nil {
			t.Fatalf("unsafe accepted: %s", value)
		}
	}
}

func TestLocalGuardAcceptsLoadtestTargets(t *testing.T) {
	values := []string{
		"http://127.0.0.1:19080",
		"postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable",
		"redis://127.0.0.1:16379/0",
		"sk-loadtestsub",
		"sk-loadtestcompat",
		"http://localhost:19080",
		"http://[::1]:19080",
		"postgres://new_api_loadtest:loadtest@localhost:15432/new_api_loadtest?sslmode=disable",
	}
	for _, value := range values {
		if err := ValidateAny(value); err != nil {
			t.Fatalf("safe rejected %s: %v", value, err)
		}
	}
}

func TestValidateListenAddrRejectsWildcard(t *testing.T) {
	for _, addr := range []string{":13080", "0.0.0.0:13080", "[::]:13080", "10.0.0.2:13080"} {
		if err := ValidateListenAddr(addr); err == nil {
			t.Fatalf("unsafe listen accepted: %s", addr)
		}
	}
}

func TestValidateLoadtestSafetyMatrixRejectsProductionInputs(t *testing.T) {
	cases := []struct{ name, value string }{
		{"client url", "https://api.openai.com"},
		{"mock url", "http://192.0.2.10:19080"},
		{"runtime url", "http://10.0.0.2:13080/debug/loadtest/runtime"},
		{"postgres default", "postgresql://new_api_loadtest:loadtest@127.0.0.1:5432/new_api_loadtest?sslmode=disable"},
		{"postgres default host port", "127.0.0.1:5432"},
		{"redis default", "redis://127.0.0.1:6379/0"},
		{"redis default host port", "127.0.0.1:6379"},
		{"listen wildcard", "0.0.0.0:13080"},
		{"real key", "sk-realproductionkey"},
	}
	for _, tc := range cases {
		if err := ValidateAny(tc.value); err == nil {
			t.Fatalf("%s accepted", tc.name)
		}
	}
}

func TestValidateCleanEnvRejectsProductionEnv(t *testing.T) {
	cases := []map[string]string{
		{"OPENAI_API_KEY": "sk-prod", "SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"},
		{"ANTHROPIC_API_KEY": "sk-prod", "SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"},
		{"AZURE_OPENAI_API_KEY": "sk-prod", "SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"},
		{"OPENAI_BASE_URL": "https://api.openai.com", "SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"},
		{"SQL_DSN": "postgresql://new_api:secret@example.com:5432/new_api", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"},
		{"SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "REDIS_CONN_STRING": "redis://127.0.0.1:6379/0"},
	}
	for _, env := range cases {
		if err := ValidateCleanEnv(env); err == nil {
			t.Fatalf("production env accepted: %#v", env)
		}
	}
}

func TestValidateCleanEnvAcceptsLoadtestEnv(t *testing.T) {
	env := map[string]string{
		"SQL_DSN":           "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable",
		"LOG_SQL_DSN":       "",
		"REDIS_CONN_STRING": "redis://127.0.0.1:16379/0",
	}
	if err := ValidateCleanEnv(env); err != nil {
		t.Fatal(err)
	}
}

func TestValidateCleanEnvDoesNotLeakSecrets(t *testing.T) {
	env := map[string]string{"SQL_DSN": "postgresql://new_api:supersecret@example.com:5432/new_api", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"}
	err := ValidateCleanEnv(env)
	if err == nil {
		t.Fatal("production env accepted")
	}
	msg := err.Error()
	for _, forbidden := range []string{"supersecret", "example.com", "postgresql://"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("error leaked %q: %v", forbidden, err)
		}
	}
}

func TestValidateCleanEnvRejectsRealProviderKeysAndURLs(t *testing.T) {
	base := map[string]string{"SQL_DSN": "postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "REDIS_CONN_STRING": "redis://127.0.0.1:16379/0"}
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "AZURE_OPENAI_API_KEY", "GEMINI_API_KEY", "OPENROUTER_API_KEY", "COHERE_API_KEY", "MISTRAL_API_KEY", "DASHSCOPE_API_KEY", "VOLCENGINE_API_KEY", "OPENAI_BASE_URL", "ANTHROPIC_BASE_URL", "AZURE_OPENAI_ENDPOINT", "GEMINI_BASE_URL", "OPENROUTER_BASE_URL"} {
		env := map[string]string{"SQL_DSN": base["SQL_DSN"], "REDIS_CONN_STRING": base["REDIS_CONN_STRING"], key: "https://api.openai.com"}
		if err := ValidateCleanEnv(env); err == nil {
			t.Fatalf("%s accepted", key)
		}
	}
}

func TestValidateCleanWorkDirRejectsDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("OPENAI_API_KEY=sk-prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCleanWorkDir(dir); err == nil {
		t.Fatal("work-dir .env accepted")
	}
}

func TestValidateLoadtestAPIKeyOnlyAllowsFixedKeys(t *testing.T) {
	for _, key := range []string{"sk-loadtestsub", "sk-loadtestcompat", "sk-loadtestinvalid"} {
		if err := ValidateLoadtestAPIKey(key); err != nil {
			t.Fatalf("loadtest key rejected: %v", err)
		}
	}
	if err := ValidateLoadtestAPIKey("sk-realproductionkey"); err == nil {
		t.Fatal("real key accepted")
	}
}

func TestRejectDefaultInfraPorts(t *testing.T) {
	if err := RejectDefaultInfraPorts("postgresql://new_api_loadtest:loadtest@127.0.0.1:5432/new_api_loadtest?sslmode=disable", "redis://127.0.0.1:16379/0"); err == nil {
		t.Fatal("postgres default port accepted")
	}
	if err := RejectDefaultInfraPorts("postgresql://new_api_loadtest:loadtest@127.0.0.1:15432/new_api_loadtest?sslmode=disable", "redis://127.0.0.1:6379/0"); err == nil {
		t.Fatal("redis default port accepted")
	}
}

package localguard

import "testing"

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

package controller

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupLoadtestRuntimeModelDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalBatchUpdateEnabled := common.BatchUpdateEnabled
	common.UsingSQLite = true
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	requireNoErrorLoadtest(t, err)
	model.DB = db
	model.LOG_DB = db
	requireNoErrorLoadtest(t, db.AutoMigrate(&model.User{}))
	t.Cleanup(func() {
		_ = model.FlushBatchUpdateTypeForMigration(model.BatchUpdateTypeUserQuota)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalUsingSQLite
		common.BatchUpdateEnabled = originalBatchUpdateEnabled
	})
}

func getLoadtestRuntimeUserQuota(t *testing.T, userID int) int {
	t.Helper()
	var user model.User
	requireNoErrorLoadtest(t, model.DB.Select("quota").First(&user, userID).Error)
	return user.Quota
}

func requireNoErrorLoadtest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadtestRuntimeRouteDisabledByDefault(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLoadtestRuntimeRouteRequiresLoopback(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden && rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLoadtestRuntimeRouteRejectsForwardedNonLoopbackClient(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rr.Code)
	}
}

func TestLoadtestRuntimeRouteNotRegisteredWhenServerNotLoopback(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	for _, listenAddr := range []string{":13080", "0.0.0.0:13080", "10.0.0.2:13080", "[::]:13080"} {
		r := gin.New()
		RegisterLoadtestRuntimeRoute(r, listenAddr, nil)
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("listen addr %q status = %d", listenAddr, rr.Code)
		}
	}
}
func TestLoadtestRuntimeRouteAllowsPrivateContainerProxyWithToken(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_RUNTIME_STATS_TOKEN", "route-secret")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, ":3000", nil)

	withoutToken := httptest.NewRecorder()
	withoutTokenReq := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	withoutTokenReq.RemoteAddr = "172.17.0.1:12345"
	r.ServeHTTP(withoutToken, withoutTokenReq)
	if withoutToken.Code != http.StatusForbidden {
		t.Fatalf("without token status = %d", withoutToken.Code)
	}

	withToken := httptest.NewRecorder()
	withTokenReq := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	withTokenReq.RemoteAddr = "172.17.0.1:12345"
	withTokenReq.Header.Set("X-New-API-Loadtest-Token", "route-secret")
	r.ServeHTTP(withToken, withTokenReq)
	if withToken.Code != http.StatusOK {
		t.Fatalf("with token status = %d body=%s", withToken.Code, withToken.Body.String())
	}
}

func TestLoadtestRuntimeRouteRejectsPublicForwardedClientWithToken(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_RUNTIME_STATS_TOKEN", "route-secret")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, ":3000", nil)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "172.17.0.1:12345"
	req.Header.Set("X-New-API-Loadtest-Token", "route-secret")
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	r.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("public forwarded status = %d", recorder.Code)
	}
}
func TestLoadtestRuntimeRouteDoesNotRegisterOnWildcardWithoutToken(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_RUNTIME_STATS_TOKEN", "")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, ":3000", nil)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "172.17.0.1:12345"
	r.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestLoadtestRuntimeDrainAllowsPrivateContainerProxyWithToken(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_RUNTIME_STATS_TOKEN", "route-secret")
	setupLoadtestRuntimeModelDB(t)
	requireNoErrorLoadtest(t, model.DB.Create(&model.User{Id: 9342, Username: "runtime-drain-token", Status: common.UserStatusEnabled}).Error)
	model.AddUserQuotaBatchForMigrationDrain(9342, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, ":3000", nil)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "172.17.0.1:12345"
	req.Header.Set(loadtestRuntimeStatsTokenHeader, "route-secret")
	r.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	assertLoadtestRuntimeDrainResponse(t, recorder.Body.Bytes(), 1, 0, "")
	if got := getLoadtestRuntimeUserQuota(t, 9342); got != 700 {
		t.Fatalf("quota = %d", got)
	}
}

func TestLoadtestRuntimeRouteReturnsRuntimeFields(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_PROFILE_BLOCK_RATE", "1000")
	t.Setenv("LOADTEST_PROFILE_MUTEX_FRACTION", "5")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	for _, want := range []string{"goroutines", "heap_alloc_bytes", "block_profile_rate", "mutex_profile_fraction", "batch_update", "quota_data", "perf_metrics", "unavailable"} {
		if !strings.Contains(rr.Body.String(), want) {
			t.Fatalf("missing %s in %s", want, rr.Body.String())
		}
	}
}

func TestLoadtestRuntimeDrainCapabilityIsReadOnly(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)
	req := httptest.NewRequest(http.MethodOptions, "/debug/loadtest/runtime/batch-update/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]any{"success": true, "state": "open", "runtime_stats": true, "full_writer_drain": true} {
		if body[key] != want {
			t.Fatalf("%s = %v want %v", key, body[key], want)
		}
	}
}

func TestLoadtestRuntimeRouteIncludesGCAndHTTPStats(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	t.Setenv("LOADTEST_PROFILE_BLOCK_RATE", "1000")
	t.Setenv("LOADTEST_PROFILE_MUTEX_FRACTION", "5")
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })
	r := gin.New()
	stats := NewLoadtestHTTPStats()
	stats.OnAccept()
	stats.OnConnState(http.StateNew)
	stats.OnConnState(http.StateActive)
	stats.OnConnState(http.StateIdle)
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", stats)
	req := httptest.NewRequest(http.MethodGet, "/debug/loadtest/runtime", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := common.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"gomaxprocs", "gomemlimit_bytes", "gc_count", "last_gc_unix_ms", "pause_total_ns", "http_conn_state", "http_accept_total", "http_active_current"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %s in %s", key, w.Body.String())
		}
	}
	if got := body["http_accept_total"]; got != float64(1) {
		t.Fatalf("http_accept_total = %v", got)
	}
	if got := body["http_active_current"]; got != float64(0) {
		t.Fatalf("http_active_current = %v", got)
	}
	connState, ok := body["http_conn_state"].(map[string]any)
	if !ok {
		t.Fatalf("http_conn_state has type %T", body["http_conn_state"])
	}
	for key, want := range map[string]float64{"new": 1, "active": 1, "idle": 1, "hijacked": 0, "closed": 0} {
		if got := connState[key]; got != want {
			t.Fatalf("http_conn_state[%s] = %v want %v", key, got, want)
		}
	}
}

func TestLoadtestRuntimeDrainUserQuotaBatchFlushesLocalPending(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	setupLoadtestRuntimeModelDB(t)
	requireNoErrorLoadtest(t, model.DB.Create(&model.User{Id: 9320, Username: "runtime-drain", Status: common.UserStatusEnabled}).Error)
	model.AddUserQuotaBatchForMigrationDrain(9320, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertLoadtestRuntimeDrainResponse(t, w.Body.Bytes(), 1, 0, "")
	if got := model.BatchUpdatePendingSnapshot().ByType[model.BatchUpdateTypeUserQuota]; got != 0 {
		t.Fatalf("pending = %d", got)
	}
	if got := getLoadtestRuntimeUserQuota(t, 9320); got != 700 {
		t.Fatalf("quota = %d", got)
	}
}

func TestLoadtestRuntimeDrainUserQuotaBatchReportsPendingWhenFlushFails(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	setupLoadtestRuntimeModelDB(t)
	model.AddUserQuotaBatchForMigrationDrain(9399, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	assertLoadtestRuntimeDrainResponse(t, w.Body.Bytes(), 1, 1, "record not found")
	if got := model.BatchUpdatePendingSnapshot().ByType[model.BatchUpdateTypeUserQuota]; got != 1 {
		t.Fatalf("pending = %d", got)
	}
	requireNoErrorLoadtest(t, model.DB.Create(&model.User{Id: 9399, Username: "runtime-drain-retry", Status: common.UserStatusEnabled}).Error)
}

func TestLoadtestRuntimeDrainUserQuotaBatchRejectsForwardedNonLoopbackClient(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	setupLoadtestRuntimeModelDB(t)
	requireNoErrorLoadtest(t, model.DB.Create(&model.User{Id: 9340, Username: "runtime-drain-forbidden", Status: common.UserStatusEnabled}).Error)
	model.AddUserQuotaBatchForMigrationDrain(9340, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.8")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := model.BatchUpdatePendingSnapshot().ByType[model.BatchUpdateTypeUserQuota]; got != 1 {
		t.Fatalf("pending = %d", got)
	}
	if got := getLoadtestRuntimeUserQuota(t, 9340); got != 0 {
		t.Fatalf("quota = %d", got)
	}
}

func TestLoadtestRuntimeDrainUserQuotaBatchRejectsForwardedChainWithNonLoopbackHop(t *testing.T) {
	t.Setenv("LOADTEST_RUNTIME_STATS_ENABLED", "true")
	setupLoadtestRuntimeModelDB(t)
	requireNoErrorLoadtest(t, model.DB.Create(&model.User{Id: 9341, Username: "runtime-drain-forwarded-chain", Status: common.UserStatusEnabled}).Error)
	model.AddUserQuotaBatchForMigrationDrain(9341, 700)
	r := gin.New()
	RegisterLoadtestRuntimeRoute(r, "127.0.0.1:13080", nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/debug/loadtest/runtime/batch-update/user-quota/drain", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "127.0.0.1, 203.0.113.8")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := model.BatchUpdatePendingSnapshot().ByType[model.BatchUpdateTypeUserQuota]; got != 1 {
		t.Fatalf("pending = %d", got)
	}
	if got := getLoadtestRuntimeUserQuota(t, 9341); got != 0 {
		t.Fatalf("quota = %d", got)
	}
}

func assertLoadtestRuntimeDrainResponse(t *testing.T, bodyBytes []byte, beforePending float64, afterPending float64, errorContains string) {
	t.Helper()
	var body map[string]any
	if err := common.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatal(err)
	}
	if got := body["flushed_type"]; got != "user_quota" {
		t.Fatalf("flushed_type = %v", got)
	}
	if _, ok := body["note"].(string); !ok {
		t.Fatalf("missing note in %#v", body)
	}
	if got := body["pending"]; got != afterPending {
		t.Fatalf("pending response = %v", got)
	}
	errorValue, ok := body["error"].(string)
	if !ok {
		t.Fatalf("error has type %T", body["error"])
	}
	if errorContains == "" {
		if errorValue != "" {
			t.Fatalf("error = %q", errorValue)
		}
	} else if !strings.Contains(errorValue, errorContains) {
		t.Fatalf("error = %q", errorValue)
	}
	assertLoadtestRuntimeDrainSnapshotPending(t, body, "before", beforePending)
	assertLoadtestRuntimeDrainSnapshotPending(t, body, "after", afterPending)
}

func assertLoadtestRuntimeDrainSnapshotPending(t *testing.T, body map[string]any, key string, want float64) {
	t.Helper()
	snapshot, ok := body[key].(map[string]any)
	if !ok {
		t.Fatalf("%s has type %T", key, body[key])
	}
	byType, ok := snapshot["by_type"].(map[string]any)
	if !ok {
		t.Fatalf("%s.by_type has type %T", key, snapshot["by_type"])
	}
	got := byType["0"]
	if got != want {
		t.Fatalf("%s.by_type[0] = %v want %v", key, got, want)
	}
}

func TestLoadtestHTTPStatsSnapshotCountsAndClampsActiveCurrent(t *testing.T) {
	stats := NewLoadtestHTTPStats()
	stats.OnConnState(http.StateIdle)
	stats.OnAccept()
	stats.OnConnState(http.StateNew)
	stats.OnConnState(http.StateActive)
	stats.OnConnState(http.StateHijacked)
	stats.OnConnState(http.StateClosed)

	connState, acceptTotal, activeCurrent := stats.Snapshot()
	if acceptTotal != 1 {
		t.Fatalf("accept total = %d", acceptTotal)
	}
	if activeCurrent != 0 {
		t.Fatalf("active current = %d", activeCurrent)
	}
	for key, want := range map[string]int64{"new": 1, "active": 1, "idle": 1, "hijacked": 1, "closed": 1} {
		if got := connState[key]; got != want {
			t.Fatalf("conn state %s = %d want %d", key, got, want)
		}
	}
}

func TestLoadtestCountingListenerCountsSuccessfulAcceptOnly(t *testing.T) {
	stats := NewLoadtestHTTPStats()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	listener := NewLoadtestCountingListener(&fakeLoadtestListener{conn: serverConn}, stats)
	accepted, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	if accepted != serverConn {
		t.Fatalf("accepted connection mismatch")
	}
	_, acceptTotal, _ := stats.Snapshot()
	if acceptTotal != 1 {
		t.Fatalf("accept total = %d", acceptTotal)
	}

	acceptErr := errors.New("accept failed")
	failingListener := NewLoadtestCountingListener(&fakeLoadtestListener{err: acceptErr}, stats)
	accepted, err = failingListener.Accept()
	if !errors.Is(err, acceptErr) || accepted != nil {
		t.Fatalf("Accept() = (%v, %v)", accepted, err)
	}
	_, acceptTotal, _ = stats.Snapshot()
	if acceptTotal != 1 {
		t.Fatalf("failed accept changed total to %d", acceptTotal)
	}

	passthrough := &fakeLoadtestListener{}
	if NewLoadtestCountingListener(passthrough, nil) != passthrough {
		t.Fatalf("nil stats should return the inner listener")
	}
}

type fakeLoadtestListener struct {
	conn net.Conn
	err  error
}

func (l *fakeLoadtestListener) Accept() (net.Conn, error) {
	if l.err != nil {
		return nil, l.err
	}
	return l.conn, nil
}

func (l *fakeLoadtestListener) Close() error {
	return nil
}

func (l *fakeLoadtestListener) Addr() net.Addr {
	return fakeLoadtestAddr("loadtest")
}

type fakeLoadtestAddr string

func (a fakeLoadtestAddr) Network() string {
	return string(a)
}

func (a fakeLoadtestAddr) String() string {
	return string(a)
}

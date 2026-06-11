package controller

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tokenAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type tokenPageResponse struct {
	Items []tokenResponseItem `json:"items"`
}

type tokenResponseItem struct {
	ID                int    `json:"id"`
	Name              string `json:"name"`
	Key               string `json:"key"`
	Status            int    `json:"status"`
	RemainQuota       int    `json:"remain_quota"`
	UsedQuota         int    `json:"used_quota"`
	UnlimitedQuota    bool   `json:"unlimited_quota"`
	TokenLimitEnabled bool   `json:"token_limit_enabled"`
	TokenLimit        int64  `json:"token_limit"`
	TokenUsed         int64  `json:"token_used"`
	TokenRemaining    int64  `json:"token_remaining"`
	TokenUnlimited    bool   `json:"token_unlimited"`
	CodexProMode      string `json:"codex_pro_mode"`
}

type tokenKeyResponse struct {
	Key string `json:"key"`
}

type sqliteColumnInfo struct {
	Name string `gorm:"column:name"`
	Type string `gorm:"column:type"`
}

type legacyToken struct {
	Id                 int    `gorm:"primaryKey"`
	UserId             int    `gorm:"index"`
	Key                string `gorm:"column:key;type:char(48);uniqueIndex"`
	Status             int    `gorm:"default:1"`
	Name               string `gorm:"index"`
	CreatedTime        int64  `gorm:"bigint"`
	AccessedTime       int64  `gorm:"bigint"`
	ExpiredTime        int64  `gorm:"bigint;default:-1"`
	RemainQuota        int    `gorm:"default:0"`
	UnlimitedQuota     bool
	ModelLimitsEnabled bool
	ModelLimits        string  `gorm:"type:text"`
	AllowIps           *string `gorm:"default:''"`
	UsedQuota          int     `gorm:"default:0"`
	Group              string  `gorm:"column:group;default:''"`
	CrossGroupRetry    bool
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

func (legacyToken) TableName() string {
	return "tokens"
}

func openTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func migrateTokenControllerTestDB(t *testing.T, db *gorm.DB) {
	t.Helper()

	if err := db.AutoMigrate(&model.Token{}, &model.Log{}, &model.TokenLimitPreConsumeRecord{}, &model.User{}); err != nil {
		t.Fatalf("failed to migrate token controller tables: %v", err)
	}
}

func setupTokenControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := openTokenControllerTestDB(t)
	migrateTokenControllerTestDB(t, db)
	return db
}

func openTokenControllerExternalDB(t *testing.T, dialect string, dsn string) (*gorm.DB, *bool) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.RedisEnabled = false
	common.UsingSQLite = false
	common.UsingMySQL = dialect == "mysql"
	common.UsingPostgreSQL = dialect == "postgres"

	var (
		db  *gorm.DB
		err error
	)
	switch dialect {
	case "mysql":
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	case "postgres":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	default:
		t.Fatalf("unsupported dialect %q", dialect)
	}
	if err != nil {
		t.Fatalf("failed to open %s db: %v", dialect, err)
	}

	model.DB = db
	model.LOG_DB = db

	if db.Migrator().HasTable("tokens") {
		t.Skipf("refusing to run %s migration compatibility test against external database because tokens table already exists", dialect)
	}

	managedTokensTable := new(bool)

	t.Cleanup(func() {
		if *managedTokensTable && db.Migrator().HasTable("tokens") {
			_ = db.Migrator().DropTable("tokens")
		}
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	return db, managedTokensTable
}

func seedToken(t *testing.T, db *gorm.DB, userID int, name string, rawKey string) *model.Token {
	t.Helper()

	token := &model.Token{
		UserId:         userID,
		Name:           name,
		Key:            rawKey,
		Status:         common.TokenStatusEnabled,
		CreatedTime:    1,
		AccessedTime:   1,
		ExpiredTime:    -1,
		RemainQuota:    100,
		UnlimitedQuota: true,
		Group:          "default",
	}
	if err := db.Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}
	return token
}

func newAuthenticatedContext(t *testing.T, method string, target string, body any, userID int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := common.Marshal(body)
		if err != nil {
			t.Fatalf("failed to marshal request body: %v", err)
		}
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, requestBody)
	if body != nil {
		ctx.Request.Header.Set("Content-Type", "application/json")
	}
	ctx.Set("id", userID)
	return ctx, recorder
}

func decodeAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder) tokenAPIResponse {
	t.Helper()

	var response tokenAPIResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode api response: %v", err)
	}
	return response
}

func decodeTokenData(t *testing.T, response tokenAPIResponse) tokenResponseItem {
	t.Helper()
	var data tokenResponseItem
	require.NoError(t, common.Unmarshal(response.Data, &data))
	return data
}

func requireTokenLimitResponse(t *testing.T, data tokenResponseItem, enabled bool, limit int64, used int64, remaining int64, unlimited bool) {
	t.Helper()
	require.Equal(t, enabled, data.TokenLimitEnabled)
	require.Equal(t, limit, data.TokenLimit)
	require.Equal(t, used, data.TokenUsed)
	require.Equal(t, remaining, data.TokenRemaining)
	require.Equal(t, unlimited, data.TokenUnlimited)
}

func getSQLiteColumnType(t *testing.T, db *gorm.DB, tableName string, columnName string) string {
	t.Helper()

	var columns []sqliteColumnInfo
	if err := db.Raw("PRAGMA table_info(" + tableName + ")").Scan(&columns).Error; err != nil {
		t.Fatalf("failed to inspect %s schema: %v", tableName, err)
	}

	for _, column := range columns {
		if column.Name == columnName {
			return strings.ToLower(column.Type)
		}
	}

	t.Fatalf("column %s not found in %s schema", columnName, tableName)
	return ""
}

func getTokenKeyColumnType(t *testing.T, db *gorm.DB, dialect string) string {
	t.Helper()

	switch dialect {
	case "sqlite":
		return getSQLiteColumnType(t, db, "tokens", "key")
	case "mysql":
		var columnType string
		if err := db.Raw(`SELECT COLUMN_TYPE FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Scan(&columnType).Error; err != nil {
			t.Fatalf("failed to inspect mysql token key column: %v", err)
		}
		return strings.ToLower(columnType)
	case "postgres":
		var dataType string
		var maxLength sql.NullInt64
		if err := db.Raw(`SELECT data_type, character_maximum_length
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			"tokens", "key").Row().Scan(&dataType, &maxLength); err != nil {
			t.Fatalf("failed to inspect postgres token key column: %v", err)
		}
		switch strings.ToLower(dataType) {
		case "character varying":
			return fmt.Sprintf("varchar(%d)", maxLength.Int64)
		case "character":
			return fmt.Sprintf("char(%d)", maxLength.Int64)
		default:
			if maxLength.Valid {
				return fmt.Sprintf("%s(%d)", strings.ToLower(dataType), maxLength.Int64)
			}
			return strings.ToLower(dataType)
		}
	default:
		t.Fatalf("unsupported dialect %q", dialect)
		return ""
	}
}

func runTokenMigrationCompatibilityTest(t *testing.T, db *gorm.DB, dialect string, managedTokensTable *bool) {
	t.Helper()

	legacyKey := strings.Repeat("a", 48)
	longKey := strings.Repeat("b", 64)

	if err := db.AutoMigrate(&legacyToken{}); err != nil {
		t.Fatalf("failed to create legacy token schema: %v", err)
	}
	if managedTokensTable != nil {
		*managedTokensTable = true
	}
	if err := db.Create(&legacyToken{
		UserId:             7,
		Key:                legacyKey,
		Status:             common.TokenStatusEnabled,
		Name:               "legacy-token",
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        100,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}).Error; err != nil {
		t.Fatalf("failed to seed legacy token row: %v", err)
	}

	if got := getTokenKeyColumnType(t, db, dialect); got != "char(48)" {
		t.Fatalf("expected legacy key column type char(48), got %q", got)
	}

	migrateTokenControllerTestDB(t, db)

	if got := getTokenKeyColumnType(t, db, dialect); got != "varchar(128)" {
		t.Fatalf("expected migrated key column type varchar(128), got %q", got)
	}

	var migratedToken model.Token
	if err := db.First(&migratedToken, "name = ?", "legacy-token").Error; err != nil {
		t.Fatalf("failed to load migrated token row: %v", err)
	}
	if migratedToken.Key != legacyKey {
		t.Fatalf("expected migrated token key %q, got %q", legacyKey, migratedToken.Key)
	}
	if migratedToken.Name != "legacy-token" {
		t.Fatalf("expected migrated token name to be preserved, got %q", migratedToken.Name)
	}

	inserted := model.Token{
		UserId:             8,
		Name:               "long-token",
		Key:                longKey,
		Status:             common.TokenStatusEnabled,
		CreatedTime:        1,
		AccessedTime:       1,
		ExpiredTime:        -1,
		RemainQuota:        200,
		UnlimitedQuota:     true,
		ModelLimitsEnabled: false,
		ModelLimits:        "",
		AllowIps:           common.GetPointer(""),
		UsedQuota:          0,
		Group:              "default",
		CrossGroupRetry:    false,
	}
	if err := db.Create(&inserted).Error; err != nil {
		t.Fatalf("failed to insert long token after migration: %v", err)
	}

	var fetched model.Token
	if err := db.First(&fetched, "id = ?", inserted.Id).Error; err != nil {
		t.Fatalf("failed to fetch long token after migration: %v", err)
	}
	if fetched.Key != longKey {
		t.Fatalf("expected long token key %q, got %q", longKey, fetched.Key)
	}
}

func TestTokenAutoMigrateUsesVarchar128KeyColumn(t *testing.T) {
	db := setupTokenControllerTestDB(t)

	if got := getTokenKeyColumnType(t, db, "sqlite"); got != "varchar(128)" {
		t.Fatalf("expected key column type varchar(128), got %q", got)
	}
}

func TestTokenMigrationFromChar48ToVarchar128(t *testing.T) {
	db := openTokenControllerTestDB(t)
	runTokenMigrationCompatibilityTest(t, db, "sqlite", nil)
}

func TestTokenMigrationFromChar48ToVarchar128MySQL(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("set TEST_MYSQL_DSN to run mysql migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "mysql", dsn)
	runTokenMigrationCompatibilityTest(t, db, "mysql", managedTokensTable)
}

func TestTokenMigrationFromChar48ToVarchar128Postgres(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set TEST_POSTGRES_DSN to run postgres migration compatibility test")
	}

	db, managedTokensTable := openTokenControllerExternalDB(t, "postgres", dsn)
	runTokenMigrationCompatibilityTest(t, db, "postgres", managedTokensTable)
}

func TestGetAllTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "list-token", "abcd1234efgh5678")
	seedToken(t, db, 2, "other-user-token", "zzzz1234yyyy5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/?p=1&size=10", nil, 1)
	GetAllTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode token page response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one token, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("list response leaked raw token key: %s", recorder.Body.String())
	}
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func TestSearchTokensMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "searchable-token", "ijkl1234mnop5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/search?keyword=searchable-token&p=1&size=10", nil, 1)
	SearchTokens(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var page tokenPageResponse
	if err := common.Unmarshal(response.Data, &page); err != nil {
		t.Fatalf("failed to decode search response: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected exactly one search result, got %d", len(page.Items))
	}
	if page.Items[0].Key != token.GetMaskedKey() {
		t.Fatalf("expected masked search key %q, got %q", token.GetMaskedKey(), page.Items[0].Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("search response leaked raw token key: %s", recorder.Body.String())
	}
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func TestGetTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "detail-token", "qrst1234uvwx5678")

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(token.Id), nil, 1)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token detail response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked detail key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("detail response leaked raw token key: %s", recorder.Body.String())
	}
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func TestAddTokenAcceptsTokenLimitFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":                "limited",
		"expired_time":        -1,
		"token_limit_enabled": true,
		"token_limit":         1000,
		"token_used":          777,
	}, 94001)

	AddToken(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	data := decodeTokenData(t, response)
	requireTokenLimitResponse(t, data, true, 1000, 0, 1000, false)
	require.Contains(t, recorder.Body.String(), "remain_quota")
	require.Contains(t, recorder.Body.String(), "used_quota")
	require.Contains(t, recorder.Body.String(), "unlimited_quota")

	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND name = ?", 94001, "limited").First(&token).Error)
	require.True(t, token.TokenLimitEnabled)
	require.Equal(t, int64(1000), token.TokenLimit)
	require.Equal(t, int64(0), token.TokenUsed)
	require.NotContains(t, recorder.Body.String(), token.GetFullKey())
}

func TestAddTokenAcceptsCodexProModeOverride(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":           "codex-pro-key",
		"expired_time":   -1,
		"codex_pro_mode": "all",
	}, 94002)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	data := decodeTokenData(t, response)
	require.Equal(t, common.CodexProModeAll, data.CodexProMode)

	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND name = ?", 94002, "codex-pro-key").First(&token).Error)
	require.Equal(t, common.CodexProModeAll, token.CodexProMode)
}

func TestAddTokenDefaultsCodexProModeToInherit(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":         "codex-pro-default",
		"expired_time": -1,
	}, 94003)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	data := decodeTokenData(t, response)
	require.Equal(t, common.CodexProModeInherit, data.CodexProMode)

	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND name = ?", 94003, "codex-pro-default").First(&token).Error)
	require.Equal(t, common.CodexProModeInherit, token.CodexProMode)
}

func TestAddTokenBlankCodexProModeDefaultsToInherit(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":           "codex-pro-blank",
		"expired_time":   -1,
		"codex_pro_mode": "   ",
	}, 94005)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	data := decodeTokenData(t, response)
	require.Equal(t, common.CodexProModeInherit, data.CodexProMode)

	var token model.Token
	require.NoError(t, db.Where("user_id = ? AND name = ?", 94005, "codex-pro-blank").First(&token).Error)
	require.Equal(t, common.CodexProModeInherit, token.CodexProMode)
}

func TestAddTokenRejectsInvalidCodexProMode(t *testing.T) {
	setupTokenControllerTestDB(t)
	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", map[string]any{
		"name":           "codex-pro-invalid",
		"expired_time":   -1,
		"codex_pro_mode": "legacy-pro",
	}, 94004)

	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.False(t, response.Success)
}

func TestGetTokenReturnsTokenLimitFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94011, "limited-token", "limitfieldkey1234")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(250),
		"remain_quota":        123,
		"used_quota":          456,
		"unlimited_quota":     false,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/"+strconv.Itoa(token.Id), nil, 94011)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetToken(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	detail := decodeTokenData(t, response)
	requireTokenLimitResponse(t, detail, true, 1000, 250, 750, false)
	require.Equal(t, 123, detail.RemainQuota)
	require.Equal(t, 456, detail.UsedQuota)
	require.False(t, detail.UnlimitedQuota)
	require.NotContains(t, recorder.Body.String(), token.GetFullKey())

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/api/token/?p=1&size=10", nil, 94011)
	GetAllTokens(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	var page tokenPageResponse
	require.NoError(t, common.Unmarshal(response.Data, &page))
	require.Len(t, page.Items, 1)
	requireTokenLimitResponse(t, page.Items[0], true, 1000, 250, 750, false)
	require.NotContains(t, recorder.Body.String(), token.GetFullKey())

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, "/api/token/search?keyword=limited-token&p=1&size=10", nil, 94011)
	SearchTokens(ctx)
	response = decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	require.NoError(t, common.Unmarshal(response.Data, &page))
	require.Len(t, page.Items, 1)
	requireTokenLimitResponse(t, page.Items[0], true, 1000, 250, 750, false)
	require.NotContains(t, recorder.Body.String(), token.GetFullKey())
}

func TestUpdateTokenCodexProModeValidationAndPreserve(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94502, "codex-pro-mode", "sk-codex-pro-mode")
	require.NoError(t, db.Model(token).Update("codex_pro_mode", common.CodexProModeAll).Error)

	omitCtx, omitRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "codex-pro-mode-updated", "expired_time": -1,
	}, 94502)
	UpdateToken(omitCtx)
	omitResponse := decodeAPIResponse(t, omitRecorder)
	require.True(t, omitResponse.Success, omitResponse.Message)
	omitData := decodeTokenData(t, omitResponse)
	require.Equal(t, common.CodexProModeAll, omitData.CodexProMode)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, common.CodexProModeAll, got.CodexProMode)

	invalidCtx, invalidRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "codex-pro-mode-updated", "expired_time": -1, "codex_pro_mode": "legacy-pro",
	}, 94502)
	UpdateToken(invalidCtx)
	invalidResponse := decodeAPIResponse(t, invalidRecorder)
	require.False(t, invalidResponse.Success)

	offCtx, offRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "codex-pro-mode-updated", "expired_time": -1, "codex_pro_mode": "off",
	}, 94502)
	UpdateToken(offCtx)
	offResponse := decodeAPIResponse(t, offRecorder)
	require.True(t, offResponse.Success, offResponse.Message)
	offData := decodeTokenData(t, offResponse)
	require.Equal(t, common.CodexProModeOff, offData.CodexProMode)

	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, common.CodexProModeOff, got.CodexProMode)
}

func TestUpdateTokenStatusOnlyPreservesCodexProMode(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94503, "codex-pro-status-only", "sk-codex-pro-status-only")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"codex_pro_mode": common.CodexProModeAll,
		"status":         common.TokenStatusExhausted,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusEnabled,
	}, 94503)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	data := decodeTokenData(t, response)
	require.Equal(t, common.CodexProModeAll, data.CodexProMode)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, common.TokenStatusEnabled, got.Status)
	require.Equal(t, common.CodexProModeAll, got.CodexProMode)
}

func TestUpdateTokenLimitValidationAndStateSwitch(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94501, "switch", "sk-switch")
	require.NoError(t, db.Model(token).Updates(map[string]any{"token_used": int64(123), "used_quota": 456}).Error)

	badCtx, badRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "switch", "expired_time": -1, "unlimited_quota": true, "token_limit_enabled": true, "token_limit": 0,
	}, 94501)
	UpdateToken(badCtx)
	badResponse := decodeAPIResponse(t, badRecorder)
	require.False(t, badResponse.Success)

	onCtx, onRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "switch", "expired_time": -1, "unlimited_quota": true, "token_limit_enabled": true, "token_limit": 500,
	}, 94501)
	UpdateToken(onCtx)
	onResponse := decodeAPIResponse(t, onRecorder)
	require.True(t, onResponse.Success, onResponse.Message)
	onData := decodeTokenData(t, onResponse)
	requireTokenLimitResponse(t, onData, true, 500, 123, 377, false)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.True(t, got.TokenLimitEnabled)
	require.Equal(t, int64(500), got.TokenLimit)
	require.Equal(t, int64(123), got.TokenUsed, "editing limit must not reset usage")
	require.Equal(t, 456, got.UsedQuota, "new token cap must not read legacy used_quota")

	offCtx, offRecorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "switch", "expired_time": -1, "unlimited_quota": true, "token_limit_enabled": false, "token_limit": 999999,
	}, 94501)
	UpdateToken(offCtx)
	offResponse := decodeAPIResponse(t, offRecorder)
	require.True(t, offResponse.Success, offResponse.Message)
	offData := decodeTokenData(t, offResponse)
	requireTokenLimitResponse(t, offData, false, 0, 123, 0, true)

	require.NoError(t, db.First(&got, token.Id).Error)
	require.False(t, got.TokenLimitEnabled)
	require.Equal(t, int64(0), got.TokenLimit)
	require.Equal(t, int64(123), got.TokenUsed, "disabling limit must not reset usage")
	require.Equal(t, 456, got.UsedQuota, "new token cap must not read legacy used_quota")
}

func TestUpdateTokenLimitPreservesLegacyQuotaWhenPayloadOmitsFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94521, "preserve-legacy-quota", "sk-preserve-legacy-quota")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"remain_quota":    777,
		"used_quota":      333,
		"unlimited_quota": false,
		"token_used":      int64(12),
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "preserve-legacy-quota", "expired_time": -1, "token_limit_enabled": true, "token_limit": 500,
	}, 94521)
	UpdateToken(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, 777, got.RemainQuota)
	require.Equal(t, 333, got.UsedQuota)
	require.False(t, got.UnlimitedQuota)
	require.True(t, got.TokenLimitEnabled)
	require.Equal(t, int64(500), got.TokenLimit)
	require.Equal(t, int64(12), got.TokenUsed)
}

func TestUpdateTokenPreservesTokenLimitWhenPayloadOmitsTokenFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94525, "preserve-token-limit", "sk-preserve-token-limit")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(77),
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", map[string]any{
		"id": token.Id, "name": "preserve-token-limit-updated", "expired_time": -1,
	}, 94525)
	UpdateToken(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.True(t, got.TokenLimitEnabled)
	require.Equal(t, int64(1000), got.TokenLimit)
	require.Equal(t, int64(77), got.TokenUsed)
	require.Equal(t, "preserve-token-limit-updated", got.Name)
}

func TestUpdateTokenStatusAllowsLegacyExhaustedWhenSubscriptionBacked(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94531, "legacy-exhausted", "sk-legacy-exhausted")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"status":          common.TokenStatusExhausted,
		"remain_quota":    0,
		"unlimited_quota": false,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusEnabled,
	}, 94531)
	UpdateToken(ctx)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, common.TokenStatusEnabled, got.Status)
	require.Equal(t, 0, got.RemainQuota)
	require.False(t, got.UnlimitedQuota)
}

func TestUpdateTokenStatusRejectsExpiredLegacyExhaustedToken(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 94541, "expired-legacy-exhausted", "sk-expired-legacy-exhausted")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"status":          common.TokenStatusExhausted,
		"expired_time":    common.GetTimestamp() - 1,
		"remain_quota":    0,
		"unlimited_quota": false,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/?status_only=true", map[string]any{
		"id": token.Id, "status": common.TokenStatusEnabled,
	}, 94541)
	UpdateToken(ctx)
	response := decodeAPIResponse(t, recorder)
	require.False(t, response.Success)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, common.TokenStatusExhausted, got.Status)
}

func TestResetTokenUsageClearsNewTokenUsedOnlyAndRecordsAudit(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 95001, "reset", "sk-reset")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(900),
		"remain_quota":        123,
		"used_quota":          456,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, fmt.Sprintf("/api/token/%d/reset-token-usage", token.Id), nil, 95001)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	ResetTokenUsage(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success, response.Message)
	data := decodeTokenData(t, response)
	requireTokenLimitResponse(t, data, true, 1000, 0, 1000, false)

	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, int64(0), got.TokenUsed)
	require.Equal(t, 123, got.RemainQuota)
	require.Equal(t, 456, got.UsedQuota)

	var audit model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ?", 95001).Order("id desc").First(&audit).Error)
	require.Contains(t, audit.Content, "reset token usage")
	require.Contains(t, audit.Other, "\"token_id\":")
	require.Contains(t, audit.Other, "\"operator_user_id\":95001")
	require.Contains(t, audit.Other, "\"before_token_used\":900")
	require.Contains(t, audit.Other, "\"after_token_used\":0")
}

func TestResetTokenUsageRejectsForeignToken(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 95012, "foreign", "sk-foreign-reset")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(700),
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, fmt.Sprintf("/api/token/%d/reset-token-usage", token.Id), nil, 95001)
	ctx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	ResetTokenUsage(ctx)

	response := decodeAPIResponse(t, recorder)
	require.False(t, response.Success)
	var got model.Token
	require.NoError(t, db.First(&got, token.Id).Error)
	require.Equal(t, int64(700), got.TokenUsed)

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND content LIKE ?", 95001, "%reset token usage%").Count(&count).Error)
	require.Equal(t, int64(0), count)
}

func TestGetTokenStatusIncludesTokenLimit(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 96001, "status", "status-token")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(250),
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/dashboard/billing/credit_grants", nil, 96001)
	ctx.Set("token_id", token.Id)
	GetTokenStatus(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, "credit_summary", body["object"])
	require.Equal(t, true, body["token_limit_enabled"])
	require.Equal(t, float64(1000), body["token_limit"])
	require.Equal(t, float64(250), body["token_used"])
	require.Equal(t, float64(750), body["token_remaining"])
	require.Equal(t, false, body["token_unlimited"])
	require.Contains(t, body, "total_granted")
	require.Contains(t, body, "total_used")
	require.Contains(t, body, "total_available")
}

func TestGetTokenUsageIncludesTokenLimit(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 96011, "usage", "usage-token")
	require.NoError(t, db.Model(token).Updates(map[string]any{
		"token_limit_enabled": true,
		"token_limit":         int64(1000),
		"token_used":          int64(250),
		"remain_quota":        300,
		"used_quota":          200,
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/usage/token/", nil, 0)
	ctx.Request.Header.Set("Authorization", "Bearer sk-usage-token")
	GetTokenUsage(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body struct {
		Code bool            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.True(t, body.Code)
	var data map[string]any
	require.NoError(t, common.Unmarshal(body.Data, &data))
	require.Equal(t, true, data["token_limit_enabled"])
	require.Equal(t, float64(1000), data["token_limit"])
	require.Equal(t, float64(250), data["token_used"])
	require.Equal(t, float64(750), data["token_remaining"])
	require.Equal(t, false, data["token_unlimited"])
	require.Equal(t, float64(500), data["total_granted"])
	require.Equal(t, float64(200), data["total_used"])
	require.Equal(t, float64(300), data["total_available"])
	require.Equal(t, float64(500), data["legacy_total_granted"])
	require.Equal(t, float64(200), data["legacy_total_used"])
	require.Equal(t, float64(300), data["legacy_total_available"])
}

func TestUpdateTokenMasksKeyInResponse(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "editable-token", "yzab1234cdef5678")

	body := map[string]any{
		"id":                   token.Id,
		"name":                 "updated-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}

	var detail tokenResponseItem
	if err := common.Unmarshal(response.Data, &detail); err != nil {
		t.Fatalf("failed to decode token update response: %v", err)
	}
	if detail.Key != token.GetMaskedKey() {
		t.Fatalf("expected masked update key %q, got %q", token.GetMaskedKey(), detail.Key)
	}
	if strings.Contains(recorder.Body.String(), token.Key) {
		t.Fatalf("update response leaked raw token key: %s", recorder.Body.String())
	}
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func assertTokenResponseOmitsLegacyGroupFields(t *testing.T, body string) {
	t.Helper()
	require.NotContains(t, body, `"group"`)
	require.NotContains(t, body, `"cross_group_retry"`)
}

func TestAddTokenIgnoresLegacyGroupPayload(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	body := map[string]any{
		"name":                 "legacy-group-token",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "vip",
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/", body, 1)
	AddToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}
	var inserted model.Token
	require.NoError(t, db.First(&inserted, "name = ?", "legacy-group-token").Error)
	require.Equal(t, "", inserted.Group)
	require.False(t, inserted.CrossGroupRetry)
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func TestUpdateTokenPreservesLegacyGroupColumnsWhenPayloadOmitsGroup(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "preserve-group-token", "preserve1234token5678")
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{"group": "paid", "cross_group_retry": true}).Error)
	body := map[string]any{
		"id":                   token.Id,
		"name":                 "preserve-group-updated",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}
	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	require.Equal(t, "paid", updated.Group)
	require.True(t, updated.CrossGroupRetry)
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func TestUpdateTokenIgnoresExplicitLegacyGroupFields(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "explicit-group-token", "explicit1234token5678")
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{"group": "default", "cross_group_retry": false}).Error)
	body := map[string]any{
		"id":                   token.Id,
		"name":                 "explicit-group-updated",
		"expired_time":         -1,
		"remain_quota":         100,
		"unlimited_quota":      true,
		"model_limits_enabled": false,
		"model_limits":         "",
		"group":                "paid",
		"cross_group_retry":    true,
	}

	ctx, recorder := newAuthenticatedContext(t, http.MethodPut, "/api/token/", body, 1)
	UpdateToken(ctx)

	response := decodeAPIResponse(t, recorder)
	if !response.Success {
		t.Fatalf("expected success response, got message: %s", response.Message)
	}
	var updated model.Token
	require.NoError(t, db.First(&updated, token.Id).Error)
	require.Equal(t, "default", updated.Group)
	require.False(t, updated.CrossGroupRetry)
	assertTokenResponseOmitsLegacyGroupFields(t, recorder.Body.String())
}

func TestGetTokenKeyRequiresOwnershipAndReturnsFullKey(t *testing.T) {
	db := setupTokenControllerTestDB(t)
	token := seedToken(t, db, 1, "owned-token", "owner1234token5678")

	authorizedCtx, authorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 1)
	authorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(authorizedCtx)

	authorizedResponse := decodeAPIResponse(t, authorizedRecorder)
	if !authorizedResponse.Success {
		t.Fatalf("expected authorized key fetch to succeed, got message: %s", authorizedResponse.Message)
	}

	var keyData tokenKeyResponse
	if err := common.Unmarshal(authorizedResponse.Data, &keyData); err != nil {
		t.Fatalf("failed to decode token key response: %v", err)
	}
	if keyData.Key != token.GetFullKey() {
		t.Fatalf("expected full key %q, got %q", token.GetFullKey(), keyData.Key)
	}

	unauthorizedCtx, unauthorizedRecorder := newAuthenticatedContext(t, http.MethodPost, "/api/token/"+strconv.Itoa(token.Id)+"/key", nil, 2)
	unauthorizedCtx.Params = gin.Params{{Key: "id", Value: strconv.Itoa(token.Id)}}
	GetTokenKey(unauthorizedCtx)

	unauthorizedResponse := decodeAPIResponse(t, unauthorizedRecorder)
	if unauthorizedResponse.Success {
		t.Fatalf("expected unauthorized key fetch to fail")
	}
	if strings.Contains(unauthorizedRecorder.Body.String(), token.Key) {
		t.Fatalf("unauthorized key response leaked raw token key: %s", unauthorizedRecorder.Body.String())
	}
}

func TestGetOpenCodeOpenAIModels(t *testing.T) {
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 1, "default", common.UserStatusEnabled)
	seedConfigGuideUser(t, db, 2, "default", common.UserStatusEnabled)
	owned := seedConfigGuideToken(t, db, 1, "ownedtoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
	foreign := seedConfigGuideToken(t, db, 2, "foreigntoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
	seedConfigGuideAbility(t, db, "default", "gpt-5.5")
	seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")
	seedConfigGuideAbility(t, db, "default", "gpt-5.5-Sys")
	seedConfigGuideAbility(t, db, "default", "not-in-metadata")
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/opencode/openai-models?token_id=%d", owned.Id), nil, 1)
	GetOpenCodeOpenAIModels(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeAPIResponse(t, recorder)
	require.True(t, response.Success)
	data := string(response.Data)
	require.Contains(t, data, "gpt-5.5")
	require.Contains(t, data, "gpt-5.4-mini")
	require.Contains(t, data, "gpt-5.5-fast")
	require.NotContains(t, data, "not-in-metadata")
	require.NotContains(t, data, "-Sys")
	require.NotContains(t, data, "omp_openai_provider_tools")

	ctx, recorder = newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/opencode/openai-models?token_id=%d", foreign.Id), nil, 1)
	GetOpenCodeOpenAIModels(ctx)
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "foreigntoken")
}

func TestGetOpenCodeOpenAIModelsRejectsAPIKeyQueryCompatibility(t *testing.T) {
	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/token/opencode/openai-models?api_key=sk-anything", nil, 1)
	GetOpenCodeOpenAIModels(ctx)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NotContains(t, recorder.Body.String(), "sk-anything")
}

func TestGetOpenCodeOpenAIModelsReusesPublicTokenStatusCodes(t *testing.T) {
	cases := []struct {
		name        string
		tokenStatus int
		userStatus  int
		expiredTime int64
		group       string
		allowIps    *string
		wantStatus  int
	}{
		{name: "disabled", tokenStatus: common.TokenStatusDisabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", wantStatus: http.StatusForbidden},
		{name: "expired status", tokenStatus: common.TokenStatusExpired, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", wantStatus: http.StatusForbidden},
		{name: "expired time", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: 1, group: "default", wantStatus: http.StatusForbidden},
		{name: "exhausted", tokenStatus: common.TokenStatusExhausted, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", wantStatus: http.StatusOK},
		{name: "user disabled", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusDisabled, expiredTime: -1, group: "default", wantStatus: http.StatusForbidden},
		{name: "deprecated group", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "gone", wantStatus: http.StatusOK},
		{name: "ip denied", tokenStatus: common.TokenStatusEnabled, userStatus: common.UserStatusEnabled, expiredTime: -1, group: "default", allowIps: common.GetPointer("10.0.0.0/8"), wantStatus: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupConfigGuideTestDB(t)
			seedConfigGuideUser(t, db, 1, "default", tc.userStatus)
			token := seedConfigGuideToken(t, db, 1, "ownedtoken", tc.tokenStatus, tc.expiredTime, tc.group, true, "", tc.allowIps)
			seedConfigGuideAbility(t, db, "default", "gpt-5.5")
			seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")
			withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{models: configGuideTestModels()})

			ctx, recorder := newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/opencode/openai-models?token_id=%d", token.Id), nil, 1)
			GetOpenCodeOpenAIModels(ctx)
			require.Equal(t, tc.wantStatus, recorder.Code, recorder.Body.String())
			require.NotContains(t, recorder.Body.String(), "ownedtoken")
		})
	}
}

func TestGetOpenCodeOpenAIModelsMetadataUnavailable(t *testing.T) {
	db := setupConfigGuideTestDB(t)
	seedConfigGuideUser(t, db, 1, "default", common.UserStatusEnabled)
	token := seedConfigGuideToken(t, db, 1, "ownedtoken", common.TokenStatusEnabled, -1, "default", true, "", nil)
	seedConfigGuideAbility(t, db, "default", "gpt-5.5")
	seedConfigGuideAbility(t, db, "default", "gpt-5.4-mini")
	withStubOpenCodeMetadataProvider(t, stubOpenCodeMetadataProvider{err: fmt.Errorf("metadata down")})

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, fmt.Sprintf("/api/token/opencode/openai-models?token_id=%d", token.Id), nil, 1)
	GetOpenCodeOpenAIModels(ctx)
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "ownedtoken")
}

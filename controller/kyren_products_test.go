package controller

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const kyrenProductsAdminUserID = 9901

type fakeKyrenAPI struct {
	createProductFunc   func(context.Context, kyrenCreateProductRequest) (*kyrenProduct, error)
	updateProductFunc   func(context.Context, string, kyrenUpdateProductRequest) (*kyrenProduct, error)
	retrieveProductFunc func(context.Context, string) (*kyrenProduct, error)
	listProductsFunc    func(context.Context, string, int, int) (*kyrenProductList, error)

	createRequests []kyrenCreateProductRequest
	updateIDs      []string
	updateRequests []kyrenUpdateProductRequest
	retrieveIDs    []string
	listStatuses   []string
}

func (f *fakeKyrenAPI) createProduct(ctx context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
	f.createRequests = append(f.createRequests, req)
	if f.createProductFunc != nil {
		return f.createProductFunc(ctx, req)
	}
	return &kyrenProduct{ID: "prod_created", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
}

func (f *fakeKyrenAPI) updateProduct(ctx context.Context, id string, req kyrenUpdateProductRequest) (*kyrenProduct, error) {
	f.updateIDs = append(f.updateIDs, id)
	f.updateRequests = append(f.updateRequests, req)
	if f.updateProductFunc != nil {
		return f.updateProductFunc(ctx, id, req)
	}
	return &kyrenProduct{ID: id, Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
}

func (f *fakeKyrenAPI) retrieveProduct(ctx context.Context, id string) (*kyrenProduct, error) {
	f.retrieveIDs = append(f.retrieveIDs, id)
	if f.retrieveProductFunc != nil {
		return f.retrieveProductFunc(ctx, id)
	}
	return nil, errors.New("unexpected retrieveProduct call")
}

func (f *fakeKyrenAPI) listProducts(ctx context.Context, status string, page int, size int) (*kyrenProductList, error) {
	f.listStatuses = append(f.listStatuses, status)
	if f.listProductsFunc != nil {
		return f.listProductsFunc(ctx, status, page, size)
	}
	return &kyrenProductList{}, nil
}

func (f *fakeKyrenAPI) createCheckout(context.Context, kyrenCreateCheckoutRequest) (*kyrenCheckoutSession, error) {
	return nil, errors.New("unexpected createCheckout call")
}

type kyrenControllerResponse[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type kyrenSubscriptionProductResponse struct {
	ProductID  string `json:"product_id"`
	Status     string `json:"status"`
	Price      string `json:"price"`
	Currency   string `json:"currency"`
	Synced     bool   `json:"synced"`
	LocalError string `json:"local_error"`
}

type kyrenTopUpProductsResponse struct {
	Products []kyrenTopUpProduct `json:"products"`
	Version  string              `json:"version"`
}

type kyrenTopUpSyncResponse struct {
	Products   []kyrenTopUpProduct `json:"products"`
	Version    string              `json:"version"`
	ProductID  string              `json:"product_id"`
	Status     string              `json:"status"`
	Price      string              `json:"price"`
	Currency   string              `json:"currency"`
	Synced     bool                `json:"synced"`
	LocalError string              `json:"local_error"`
}

func setupKyrenProductsControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.SubscriptionPlan{}, &model.Option{}, &model.Log{}))
	require.NoError(t, db.Create(&model.User{Id: kyrenProductsAdminUserID, Username: "kyren-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled, AffCode: "kyren-admin"}).Error)

	originalOptionMap := common.OptionMap
	originalTopUps := setting.KyrenTopUpProducts
	common.OptionMapRWMutex.Lock()
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	setting.KyrenTopUpProducts = "[]"
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptionMap
		common.OptionMapRWMutex.Unlock()
		setting.KyrenTopUpProducts = originalTopUps
	})
	return db
}

func withFakeKyrenControllerClient(t *testing.T, fake *fakeKyrenAPI) {
	t.Helper()
	original := newKyrenClientForController
	newKyrenClientForController = func() (kyrenAPI, error) { return fake, nil }
	t.Cleanup(func() { newKyrenClientForController = original })
}

func performKyrenControllerRequest(t *testing.T, method string, target string, body any, params gin.Params, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := common.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(payload)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, reader)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = params
	ctx.Set("id", kyrenProductsAdminUserID)
	ctx.Set("username", "kyren-admin")
	ctx.Set("role", common.RoleAdminUser)
	handler(ctx)
	return recorder
}

func decodeKyrenControllerResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder) kyrenControllerResponse[T] {
	t.Helper()
	var response kyrenControllerResponse[T]
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return response
}

func seedKyrenSubscriptionPlan(t *testing.T, id int, productID string) model.SubscriptionPlan {
	t.Helper()
	businessCode := fmt.Sprintf("kyren_plan_%d", id)
	plan := model.SubscriptionPlan{
		Id:                 id,
		Title:              "Kyren Basic",
		PriceAmount:        40,
		Currency:           kyrenCurrencyCNY,
		DurationUnit:       model.SubscriptionDurationMonth,
		DurationValue:      1,
		Enabled:            true,
		PublicVisible:      true,
		MonthlyTokenLimit:  1000,
		ConcurrencyLimit:   1,
		QueueCapacity:      1,
		BusinessCode:       &businessCode,
		KyrenProductId:     productID,
		RewardEligible:     true,
		MaxPurchasePerUser: 0,
	}
	require.NoError(t, model.DB.Create(&plan).Error)
	return plan
}

func setKyrenTopUpProductsOptionForTest(t *testing.T, products []kyrenTopUpProduct) string {
	t.Helper()
	payload, err := common.Marshal(products)
	require.NoError(t, err)
	normalized, err := normalizeKyrenTopUpProductsJSON(string(payload))
	require.NoError(t, err)
	option := model.Option{Key: "KyrenTopUpProducts"}
	require.NoError(t, model.DB.FirstOrCreate(&option, model.Option{Key: "KyrenTopUpProducts"}).Error)
	option.Value = normalized
	require.NoError(t, model.DB.Save(&option).Error)
	setting.KyrenTopUpProducts = normalized
	common.OptionMapRWMutex.Lock()
	common.OptionMap["KyrenTopUpProducts"] = normalized
	common.OptionMapRWMutex.Unlock()
	return normalized
}

func loadKyrenTopUpProductsOptionForTest(t *testing.T) []kyrenTopUpProduct {
	t.Helper()
	var option model.Option
	require.NoError(t, kyrenTopUpProductsOptionQuery(model.DB).First(&option).Error)
	var products []kyrenTopUpProduct
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &products))
	return products
}

func kyrenTopUpProductFixture(id string, productID string) kyrenTopUpProduct {
	return kyrenTopUpProduct{
		ID:          id,
		Name:        "充值 " + id,
		Description: "充值档位 " + id,
		ProductID:   productID,
		Amount:      "10.00",
		Currency:    kyrenCurrencyCNY,
		Quota:       5000000,
		Enabled:     true,
	}
}

func assertLatestManageLogHasAdminInfo(t *testing.T) {
	t.Helper()
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeManage).Order("id desc").First(&log).Error)
	assert.Equal(t, kyrenProductsAdminUserID, log.UserId)
	var other map[string]any
	require.NoError(t, common.Unmarshal([]byte(log.Other), &other))
	adminInfo, ok := other["admin_info"].(map[string]any)
	require.True(t, ok, "admin_info must be present in log.Other: %s", log.Other)
	assert.Equal(t, float64(kyrenProductsAdminUserID), adminInfo["admin_id"])
	assert.Equal(t, "kyren-admin", adminInfo["admin_username"])
}

func TestKyrenTopUpProductsOptionQueriesQuoteOptionKeyColumn(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)

	stmt := kyrenTopUpProductsOptionQuery(model.DB.Session(&gorm.Session{DryRun: true})).Find(&model.Option{}).Statement
	sql := stmt.SQL.String()

	assert.NotContains(t, sql, "WHERE key =")
	assert.True(t, strings.Contains(sql, "`key`") || strings.Contains(sql, `"key"`), sql)
}
func TestAdminSyncSubscriptionKyrenProductCreatesAndBindsProduct(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	plan := seedKyrenSubscriptionPlan(t, 3001, "")
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		assert.Equal(t, plan.Title, req.Name)
		assert.Equal(t, "40.00", req.Price)
		assert.Equal(t, kyrenCurrencyCNY, req.Currency)
		assert.Equal(t, "new-api", req.Metadata["source"])
		assert.Equal(t, "subscription_plan", req.Metadata["kind"])
		assert.Equal(t, strconv.Itoa(plan.Id), req.Metadata["plan_id"])
		return &kyrenProduct{ID: "prod_sub", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/subscription/admin/plans/3001/kyren/product", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "3001"}}, AdminSyncSubscriptionKyrenProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenSubscriptionProductResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "prod_sub", response.Data.ProductID)
	assert.Len(t, fake.createRequests, 1)
	var saved model.SubscriptionPlan
	require.NoError(t, model.DB.First(&saved, plan.Id).Error)
	assert.Equal(t, "prod_sub", saved.KyrenProductId)
}

func TestAdminSyncSubscriptionKyrenProductReturnsProductIDWhenLocalBindFails(t *testing.T) {
	db := setupKyrenProductsControllerTestDB(t)
	seedKyrenSubscriptionPlan(t, 3002, "")
	sqlDB, err := db.DB()
	require.NoError(t, err)
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		require.NoError(t, sqlDB.Close())
		return &kyrenProduct{ID: "prod_bind_failed", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/subscription/admin/plans/3002/kyren/product", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "3002"}}, AdminSyncSubscriptionKyrenProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenSubscriptionProductResponse](t, recorder)
	assert.False(t, response.Success)
	assert.Equal(t, "prod_bind_failed", response.Data.ProductID)
	assert.NotEmpty(t, response.Data.LocalError)
	assert.Contains(t, response.Message, "prod_bind_failed")
}

func TestAdminSyncSubscriptionKyrenProductReusesMetadataMatchedProduct(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	plan := seedKyrenSubscriptionPlan(t, 3003, "")
	fake := &fakeKyrenAPI{listProductsFunc: func(_ context.Context, status string, page int, size int) (*kyrenProductList, error) {
		return &kyrenProductList{Items: []kyrenProduct{{ID: "prod_sub_reused", Status: "ACTIVE", Price: "40.00", Currency: kyrenCurrencyCNY, Metadata: map[string]string{"source": "new-api", "kind": "subscription_plan", "plan_id": strconv.Itoa(plan.Id)}}}}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/subscription/admin/plans/3003/kyren/product", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "3003"}}, AdminSyncSubscriptionKyrenProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenSubscriptionProductResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "prod_sub_reused", response.Data.ProductID)
	assert.Empty(t, fake.createRequests)
	var saved model.SubscriptionPlan
	require.NoError(t, model.DB.First(&saved, plan.Id).Error)
	assert.Equal(t, "prod_sub_reused", saved.KyrenProductId)
}
func TestAdminSyncSubscriptionKyrenProductUpdatesExistingProduct(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	plan := seedKyrenSubscriptionPlan(t, 3005, "prod_sub_existing")
	fake := &fakeKyrenAPI{retrieveProductFunc: func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: "ACTIVE", Price: "39.00", Currency: kyrenCurrencyCNY}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/subscription/admin/plans/3005/kyren/product", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "3005"}}, AdminSyncSubscriptionKyrenProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenSubscriptionProductResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "prod_sub_existing", response.Data.ProductID)
	assert.Equal(t, []string{"prod_sub_existing"}, fake.retrieveIDs)
	require.Len(t, fake.updateIDs, 1)
	assert.Equal(t, "prod_sub_existing", fake.updateIDs[0])
	require.Len(t, fake.updateRequests, 1)
	update := fake.updateRequests[0]
	assert.Equal(t, plan.Title, update.Name)
	assert.Equal(t, "40.00", update.Price)
	assert.Equal(t, kyrenCurrencyCNY, update.Currency)
	assert.Equal(t, "subscription_plan", update.Metadata["kind"])
	assert.Empty(t, fake.createRequests)
}

func TestAdminSyncSubscriptionKyrenProductRejectsArchivedExistingProduct(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	seedKyrenSubscriptionPlan(t, 3006, "prod_sub_archived")
	fake := &fakeKyrenAPI{retrieveProductFunc: func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: "ARCHIVED", Price: "40.00", Currency: kyrenCurrencyCNY}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/subscription/admin/plans/3006/kyren/product", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "3006"}}, AdminSyncSubscriptionKyrenProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenSubscriptionProductResponse](t, recorder)
	assert.False(t, response.Success)
	assert.Contains(t, response.Message, "不可用")
	assert.Empty(t, fake.createRequests)
	var saved model.SubscriptionPlan
	require.NoError(t, model.DB.First(&saved, 3006).Error)
	assert.Equal(t, "prod_sub_archived", saved.KyrenProductId)
}

func TestAdminSyncSubscriptionKyrenProductWritesManageLog(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	seedKyrenSubscriptionPlan(t, 3004, "")
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		return &kyrenProduct{ID: "prod_sub_log", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/subscription/admin/plans/3004/kyren/product", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "3004"}}, AdminSyncSubscriptionKyrenProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenSubscriptionProductResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assertLatestManageLogHasAdminInfo(t)
}

func TestAdminUpdateKyrenTopUpProductsRejectsStaleVersion(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	originalProducts := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", "")}
	setKyrenTopUpProductsOptionForTest(t, originalProducts)

	getRecorder := performKyrenControllerRequest(t, http.MethodGet, "/api/payment/kyren/topup-products", nil, nil, AdminListKyrenTopUpProducts)
	require.Equal(t, http.StatusOK, getRecorder.Code, getRecorder.Body.String())
	getResponse := decodeKyrenControllerResponse[kyrenTopUpProductsResponse](t, getRecorder)
	require.True(t, getResponse.Success, getResponse.Message)

	latestProducts := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", "")}
	latestProducts[0].Name = "充值 topup_a latest"
	setKyrenTopUpProductsOptionForTest(t, latestProducts)

	staleProducts := getResponse.Data.Products
	staleProducts[0].Name = "stale overwrite"
	putRecorder := performKyrenControllerRequest(t, http.MethodPut, "/api/payment/kyren/topup-products", map[string]any{"version": getResponse.Data.Version, "products": staleProducts}, nil, AdminUpdateKyrenTopUpProducts)

	require.Equal(t, http.StatusConflict, putRecorder.Code, putRecorder.Body.String())
	putResponse := decodeKyrenControllerResponse[kyrenTopUpProductsResponse](t, putRecorder)
	assert.False(t, putResponse.Success)
	saved := loadKyrenTopUpProductsOptionForTest(t)
	require.Len(t, saved, 1)
	assert.Equal(t, "充值 topup_a latest", saved[0].Name)
}

func TestSaveKyrenTopUpProductsOptionCASRejectsChangedVersion(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	originalProducts := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", "")}
	originalNormalized := setKyrenTopUpProductsOptionForTest(t, originalProducts)
	originalVersion := kyrenTopUpProductsVersion(originalNormalized)

	latestProducts := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", "")}
	latestProducts[0].Name = "充值 topup_a latest"
	latestNormalized := setKyrenTopUpProductsOptionForTest(t, latestProducts)

	staleProducts := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", "")}
	staleProducts[0].Name = "stale overwrite"
	payload, err := common.Marshal(staleProducts)
	require.NoError(t, err)
	staleNormalized, err := normalizeKyrenTopUpProductsJSON(string(payload))
	require.NoError(t, err)

	currentNormalized, _, conflicted, err := saveKyrenTopUpProductsOptionCAS(originalVersion, staleNormalized)

	require.NoError(t, err)
	assert.True(t, conflicted)
	assert.Equal(t, kyrenTopUpProductsVersion(latestNormalized), kyrenTopUpProductsVersion(currentNormalized))
	saved := loadKyrenTopUpProductsOptionForTest(t)
	require.Len(t, saved, 1)
	assert.Equal(t, "充值 topup_a latest", saved[0].Name)
}

func TestSaveKyrenTopUpProductsOptionCASCreatesMissingOptionRowOnConflict(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	require.NoError(t, kyrenTopUpProductsOptionQuery(model.DB).Delete(&model.Option{}).Error)
	setting.KyrenTopUpProducts = "[]"
	common.OptionMapRWMutex.Lock()
	delete(common.OptionMap, "KyrenTopUpProducts")
	common.OptionMapRWMutex.Unlock()

	products := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_missing_row", "")}
	payload, err := common.Marshal(products)
	require.NoError(t, err)
	normalized, err := normalizeKyrenTopUpProductsJSON(string(payload))
	require.NoError(t, err)

	currentNormalized, currentProducts, conflicted, err := saveKyrenTopUpProductsOptionCAS("stale-version", normalized)

	require.NoError(t, err)
	assert.True(t, conflicted)
	assert.Equal(t, "[]", currentNormalized)
	assert.Empty(t, currentProducts)
	var option model.Option
	require.NoError(t, kyrenTopUpProductsOptionQuery(model.DB).First(&option).Error)
	assert.Equal(t, "[]", option.Value)
}

func TestAdminUpdateKyrenTopUpProductsWritesManageLog(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{})
	getRecorder := performKyrenControllerRequest(t, http.MethodGet, "/api/payment/kyren/topup-products", nil, nil, AdminListKyrenTopUpProducts)
	getResponse := decodeKyrenControllerResponse[kyrenTopUpProductsResponse](t, getRecorder)
	require.True(t, getResponse.Success, getResponse.Message)

	products := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_log", "")}
	putRecorder := performKyrenControllerRequest(t, http.MethodPut, "/api/payment/kyren/topup-products", map[string]any{"version": getResponse.Data.Version, "products": products}, nil, AdminUpdateKyrenTopUpProducts)

	require.Equal(t, http.StatusOK, putRecorder.Code, putRecorder.Body.String())
	putResponse := decodeKyrenControllerResponse[kyrenTopUpProductsResponse](t, putRecorder)
	require.True(t, putResponse.Success, putResponse.Message)
	assertLatestManageLogHasAdminInfo(t)
}

func TestAdminSyncKyrenTopUpProductMergesLatestOptionValue(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	products := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", ""), kyrenTopUpProductFixture("topup_b", "")}
	setKyrenTopUpProductsOptionForTest(t, products)
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		latest := []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_a", ""), kyrenTopUpProductFixture("topup_b", "")}
		latest[1].Name = "充值 topup_b latest"
		setKyrenTopUpProductsOptionForTest(t, latest)
		return &kyrenProduct{ID: "prod_topup_a", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/payment/kyren/topup-products/topup_a/sync", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "topup_a"}}, AdminSyncKyrenTopUpProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenTopUpSyncResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	saved := loadKyrenTopUpProductsOptionForTest(t)
	require.Len(t, saved, 2)
	assert.Equal(t, "prod_topup_a", saved[0].ProductID)
	assert.Equal(t, "充值 topup_b latest", saved[1].Name)
}

func TestAdminSyncKyrenTopUpProductUpdatesExistingProduct(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	product := kyrenTopUpProductFixture("topup_update", "prod_existing")
	product.Amount = "12.50"
	product.Description = "updated description"
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{product})
	fake := &fakeKyrenAPI{retrieveProductFunc: func(_ context.Context, id string) (*kyrenProduct, error) {
		return &kyrenProduct{ID: id, Status: "ACTIVE", Price: "10.00", Currency: kyrenCurrencyCNY}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/payment/kyren/topup-products/topup_update/sync", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "topup_update"}}, AdminSyncKyrenTopUpProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenTopUpSyncResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, []string{"prod_existing"}, fake.retrieveIDs)
	require.Len(t, fake.updateIDs, 1)
	assert.Equal(t, "prod_existing", fake.updateIDs[0])
	require.Len(t, fake.updateRequests, 1)
	update := fake.updateRequests[0]
	assert.Equal(t, product.Name, update.Name)
	assert.Equal(t, product.Description, update.Description)
	assert.Equal(t, "12.50", update.Price)
	assert.Equal(t, kyrenCurrencyCNY, update.Currency)
	assert.Equal(t, "new-api", update.Metadata["source"])
	assert.Equal(t, "wallet_topup", update.Metadata["kind"])
	assert.Equal(t, product.ID, update.Metadata["topup_product_id"])
	assert.Empty(t, fake.createRequests)
}

func TestAdminSyncKyrenTopUpProductReturnsProductIDWhenOptionSaveFails(t *testing.T) {
	db := setupKyrenProductsControllerTestDB(t)
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_fail", "")})
	sqlDB, err := db.DB()
	require.NoError(t, err)
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		require.NoError(t, sqlDB.Close())
		return &kyrenProduct{ID: "prod_option_failed", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/payment/kyren/topup-products/topup_fail/sync", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "topup_fail"}}, AdminSyncKyrenTopUpProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenTopUpSyncResponse](t, recorder)
	assert.False(t, response.Success)
	assert.Equal(t, "prod_option_failed", response.Data.ProductID)
	assert.NotEmpty(t, response.Data.LocalError)
	assert.Contains(t, response.Message, "prod_option_failed")
}

func TestAdminSyncKyrenTopUpProductReusesMetadataMatchedProduct(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_reuse", "")})
	fake := &fakeKyrenAPI{listProductsFunc: func(_ context.Context, status string, page int, size int) (*kyrenProductList, error) {
		return &kyrenProductList{Items: []kyrenProduct{{ID: "prod_topup_reused", Status: "ACTIVE", Price: "10.00", Currency: kyrenCurrencyCNY, Metadata: map[string]string{"source": "new-api", "kind": "wallet_topup", "topup_product_id": "topup_reuse"}}}}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/payment/kyren/topup-products/topup_reuse/sync", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "topup_reuse"}}, AdminSyncKyrenTopUpProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenTopUpSyncResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "prod_topup_reused", response.Data.ProductID)
	assert.Empty(t, fake.createRequests)
	saved := loadKyrenTopUpProductsOptionForTest(t)
	require.Len(t, saved, 1)
	assert.Equal(t, "prod_topup_reused", saved[0].ProductID)
}

func TestAdminSyncKyrenTopUpProductWritesManageLog(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_log", "")})
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		return &kyrenProduct{ID: "prod_topup_log", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/payment/kyren/topup-products/topup_log/sync", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "topup_log"}}, AdminSyncKyrenTopUpProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenTopUpSyncResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assertLatestManageLogHasAdminInfo(t)
}

func TestAdminSyncKyrenTopUpProductReturnsLatestProductsAndVersion(t *testing.T) {
	setupKyrenProductsControllerTestDB(t)
	setKyrenTopUpProductsOptionForTest(t, []kyrenTopUpProduct{kyrenTopUpProductFixture("topup_latest", "")})
	fake := &fakeKyrenAPI{createProductFunc: func(_ context.Context, req kyrenCreateProductRequest) (*kyrenProduct, error) {
		return &kyrenProduct{ID: "prod_topup_latest", Status: "ACTIVE", Price: req.Price, Currency: req.Currency, Metadata: req.Metadata}, nil
	}}
	withFakeKyrenControllerClient(t, fake)

	recorder := performKyrenControllerRequest(t, http.MethodPost, "/api/payment/kyren/topup-products/topup_latest/sync", map[string]any{"mode": "create_or_update"}, gin.Params{{Key: "id", Value: "topup_latest"}}, AdminSyncKyrenTopUpProduct)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	response := decodeKyrenControllerResponse[kyrenTopUpSyncResponse](t, recorder)
	require.True(t, response.Success, response.Message)
	assert.Equal(t, "prod_topup_latest", response.Data.ProductID)
	require.Len(t, response.Data.Products, 1)
	assert.Equal(t, "prod_topup_latest", response.Data.Products[0].ProductID)
	latestPayload, err := common.Marshal(response.Data.Products)
	require.NoError(t, err)
	normalized, err := normalizeKyrenTopUpProductsJSON(string(latestPayload))
	require.NoError(t, err)
	assert.Equal(t, kyrenTopUpProductsVersion(normalized), response.Data.Version)
}

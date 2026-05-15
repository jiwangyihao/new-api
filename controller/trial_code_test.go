package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminListTrialCodesFiltersBeforePagination(t *testing.T) {
	setupPasswordRegisterTrialTest(t)
	plan := seedControllerTrialPlan(t, 7831, "trial_filter")
	require.NoError(t, model.DB.Create(&model.TrialCode{Id: 7832, Code: "ALPHA", PlanId: plan.Id, Enabled: true}).Error)
	require.NoError(t, model.DB.Create(&model.TrialCode{Id: 7833, Code: "OMEGA", PlanId: plan.Id, Enabled: true}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/trial-codes/admin?filter=ALPHA&p=1&page_size=1", nil)

	AdminListTrialCodes(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"total":1`)
	assert.Contains(t, recorder.Body.String(), `"code":"ALPHA"`)
	assert.NotContains(t, recorder.Body.String(), `"code":"OMEGA"`)
}

package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetTrialAbuseSummary(c *gin.Context) {
	query, err := parseTrialAbuseSummaryQuery(c)
	if err != nil {
		writeTrialAbuseBadRequest(c, err.Error())
		return
	}
	data, err := service.GetTrialAbuseSummary(c.Request.Context(), query)
	if err != nil {
		if service.IsTrialAbuseInvalidQueryError(err) {
			writeTrialAbuseBadRequest(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, data)
}

func parseTrialAbuseSummaryQuery(c *gin.Context) (service.TrialAbuseSummaryQuery, error) {
	var err error
	query := service.TrialAbuseSummaryQuery{}
	if query.TrialEndStart, err = parseTrialAbuseOptionalInt64(c, "trial_end_start"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.TrialEndEnd, err = parseTrialAbuseOptionalInt64(c, "trial_end_end"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.RegisteredStart, err = parseTrialAbuseOptionalInt64(c, "registered_start"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.RegisteredEnd, err = parseTrialAbuseOptionalInt64(c, "registered_end"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.SnapshotAt, err = parseTrialAbuseOptionalInt64(c, "snapshot_at"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.MinConsumeCount, err = parseTrialAbuseOptionalInt(c, "min_consume_count"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.MinClusterSize, err = parseTrialAbuseOptionalInt(c, "min_cluster_size"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.RiskLimit, err = parseTrialAbuseOptionalInt(c, "risk_limit"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	if query.GroupLimit, err = parseTrialAbuseOptionalInt(c, "group_limit"); err != nil {
		return service.TrialAbuseSummaryQuery{}, err
	}
	return query, nil
}

func parseTrialAbuseOptionalInt64(c *gin.Context, key string) (int64, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errors.New("invalid " + key)
	}
	return value, nil
}

func parseTrialAbuseOptionalInt(c *gin.Context, key string) (int, error) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("invalid " + key)
	}
	return value, nil
}

func writeTrialAbuseBadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": message})
}

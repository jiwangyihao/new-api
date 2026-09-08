package service

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// AttachMidjourneyAvailability keeps accepted async work pending until the
// persisted task reaches a terminal status. Provider rejection is not success.
func AttachMidjourneyAvailability(c *gin.Context, task *model.Midjourney) {
	if task.Code != 1 && task.Code != 21 && task.Code != 22 {
		ObserveAvailabilityTaskError(c, "drawing_rejected", http.StatusBadGateway, false)
		if task.Code == 24 {
			ObserveAvailabilityTaskError(c, "prompt_blocked", http.StatusBadRequest, true)
		}
		return
	}
	observation := DeferTaskAvailability(c)
	if observation == nil {
		return
	}
	data, err := common.Marshal(observation)
	if err != nil {
		AvailabilityTaskInsertFailed(c)
		return
	}
	task.AvailabilityData = string(data)
	task.AvailabilityPending = true
}

// ObserveMidjourneyAvailabilityError classifies known local request and provider
// policy rejections without exposing raw provider descriptions in observations.
func ObserveMidjourneyAvailabilityError(c *gin.Context, code int, description string) {
	switch description {
	case "bind_request_body_failed", "sour_base64_and_target_base64_is_required", "prompt_is_required", "invalid_action":
		ObserveAvailabilityTaskError(c, description, http.StatusBadRequest, true)
	default:
		ObserveAvailabilityTaskError(c, "drawing_error_"+strconv.Itoa(code), http.StatusBadGateway, false)
	}
	if code == 24 {
		ObserveAvailabilityTaskError(c, "prompt_blocked", http.StatusBadRequest, true)
	}
}

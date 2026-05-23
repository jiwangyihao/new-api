package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPricingRemovesBusinessGroupFieldsForAllAudiences(t *testing.T) {
	setupPricingDirectoryTestDB(t)
	seedPricingDirectoryData(t)

	for name, userID := range map[string]*int{
		"anonymous": nil,
		"user":      intPtrForPricingDirectoryAudienceTest(9101),
		"admin":     intPtrForPricingDirectoryAudienceTest(9102),
	} {
		t.Run(name, func(t *testing.T) {
			payload := performGetPricingForDirectoryTest(t, userID)
			assert.NotContains(t, payload, "group_ratio")
			assert.NotContains(t, payload, "auto_groups")
			assert.NotContains(t, payload, "usable_group")
			item := firstPricingDirectoryItem(t, payload)
			assert.NotContains(t, item, "enable_groups")
		})
	}
}

func intPtrForPricingDirectoryAudienceTest(value int) *int {
	return &value
}

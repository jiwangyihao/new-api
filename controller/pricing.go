package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

func filterPricingByUsableGroups(pricing []model.Pricing, usableGroup map[string]string) []model.Pricing {
	if len(pricing) == 0 {
		return pricing
	}
	if len(usableGroup) == 0 {
		return []model.Pricing{}
	}

	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroup[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

type pricingDirectoryItem struct {
	ModelName              string                  `json:"model_name"`
	Description            string                  `json:"description,omitempty"`
	Icon                   string                  `json:"icon,omitempty"`
	Tags                   string                  `json:"tags,omitempty"`
	VendorID               int                     `json:"vendor_id,omitempty"`
	QuotaType              int                     `json:"quota_type"`
	EnableGroup            []string                `json:"enable_groups"`
	SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
	PricingVersion         string                  `json:"pricing_version,omitempty"`
}

func toPricingDirectoryItems(pricing []model.Pricing) []pricingDirectoryItem {
	items := make([]pricingDirectoryItem, 0, len(pricing))
	for _, item := range pricing {
		items = append(items, pricingDirectoryItem{
			ModelName:              item.ModelName,
			Description:            item.Description,
			Icon:                   item.Icon,
			Tags:                   item.Tags,
			VendorID:               item.VendorID,
			QuotaType:              item.QuotaType,
			EnableGroup:            item.EnableGroup,
			SupportedEndpointTypes: item.SupportedEndpointTypes,
			PricingVersion:         item.PricingVersion,
		})
	}
	return items
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	usableGroup := map[string]string{}
	groupRatio := map[string]float64{}
	for s, f := range ratio_setting.GetGroupRatioCopy() {
		groupRatio[s] = f
	}
	var group string
	isAdmin := false
	if exists {
		if id, ok := userId.(int); ok {
			user, err := model.GetUserById(id, false)
			if err == nil {
				group = user.Group
				isAdmin = user.Role >= common.RoleAdminUser
				for g := range groupRatio {
					ratio, ok := ratio_setting.GetGroupGroupRatio(group, g)
					if ok {
						groupRatio[g] = ratio
					}
				}
			}
		}
	}

	usableGroup = service.GetUserUsableGroups(group)
	pricing = filterPricingByUsableGroups(pricing, usableGroup)
	// check groupRatio contains usableGroup
	for group := range ratio_setting.GetGroupRatioCopy() {
		if _, ok := usableGroup[group]; !ok {
			delete(groupRatio, group)
		}
	}

	response := gin.H{
		"success":            true,
		"data":               toPricingDirectoryItems(pricing),
		"vendors":            model.GetVendors(),
		"usable_group":       usableGroup,
		"supported_endpoint": model.GetSupportedEndpointMap(),
		"auto_groups":        service.GetUserAutoGroup(group),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	}
	if isAdmin {
		response["data"] = pricing
		response["group_ratio"] = groupRatio
	}
	c.JSON(200, response)
}

func ResetModelRatio(c *gin.Context) {
	defaultStr := ratio_setting.DefaultModelRatio2JSONString()
	err := model.UpdateOption("ModelRatio", defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	err = ratio_setting.UpdateModelRatioByJSONString(defaultStr)
	if err != nil {
		c.JSON(200, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"success": true,
		"message": "重置模型倍率成功",
	})
}

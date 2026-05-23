package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

type pricingDirectoryItem struct {
	ModelName              string                  `json:"model_name"`
	Description            string                  `json:"description,omitempty"`
	Icon                   string                  `json:"icon,omitempty"`
	Tags                   string                  `json:"tags,omitempty"`
	VendorID               int                     `json:"vendor_id,omitempty"`
	QuotaType              int                     `json:"quota_type"`
	SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
	PricingVersion         string                  `json:"pricing_version,omitempty"`
}

func toPricingDirectoryItems(pricing []model.Pricing, supportedByModel map[string][]constant.EndpointType) []pricingDirectoryItem {
	items := make([]pricingDirectoryItem, 0, len(pricing))
	for _, item := range pricing {
		endpoints := item.SupportedEndpointTypes
		if supportedByModel != nil {
			endpoints = supportedByModel[item.ModelName]
		}
		items = append(items, pricingDirectoryItem{
			ModelName:              item.ModelName,
			Description:            item.Description,
			Icon:                   item.Icon,
			Tags:                   item.Tags,
			VendorID:               item.VendorID,
			QuotaType:              item.QuotaType,
			SupportedEndpointTypes: endpoints,
			PricingVersion:         item.PricingVersion,
		})
	}
	return items
}

func GetPricing(c *gin.Context) {
	pricing := model.GetPricing()
	userId, exists := c.Get("id")
	isAdmin := false
	supportedByModel := map[string][]constant.EndpointType(nil)
	if exists {
		if id, ok := userId.(int); ok {
			user, err := model.GetUserById(id, false)
			if err == nil {
				isAdmin = user.Role >= common.RoleAdminUser
			}
		}
	}
	if !isAdmin {
		supportedByModel = model.GetPublicPricingSupportedEndpointsByModel()
	}

	response := gin.H{
		"success":            true,
		"data":               toPricingDirectoryItems(pricing, supportedByModel),
		"vendors":            model.GetVendors(),
		"supported_endpoint": model.GetSupportedEndpointMapForTypes(model.MergeEndpointTypes(supportedByModel)),
		"pricing_version":    "a42d372ccf0b5dd13ecf71203521f9d2",
	}
	if isAdmin {
		response["data"] = pricing
		response["supported_endpoint"] = model.GetSupportedEndpointMap()
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

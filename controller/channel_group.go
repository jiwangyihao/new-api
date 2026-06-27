package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// channelGroupDTO 是分组的管理端视图（含成员渠道 id）。
type channelGroupDTO struct {
	*model.ChannelGroup
	ChannelIds []int `json:"channel_ids"`
	IsDefault  bool  `json:"is_default"`
}

// availableChannelGroupDTO 是用户侧的脱敏视图：绝不含渠道信息。
type availableChannelGroupDTO struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func buildChannelGroupDTO(group *model.ChannelGroup) (*channelGroupDTO, error) {
	ids, err := model.GetChannelIdsByGroup(group.Id)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []int{}
	}
	return &channelGroupDTO{ChannelGroup: group, ChannelIds: ids, IsDefault: group.IsDefault()}, nil
}

// GetChannelGroups 管理端：列出所有分组（含成员渠道 id）。
func GetChannelGroups(c *gin.Context) {
	groups, err := model.GetAllChannelGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	dtos := make([]*channelGroupDTO, 0, len(groups))
	for _, g := range groups {
		dto, err := buildChannelGroupDTO(g)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		dtos = append(dtos, dto)
	}
	common.ApiSuccess(c, dtos)
}

type channelGroupPayload struct {
	model.ChannelGroup
	ChannelIds []int `json:"channel_ids"`
}

// CreateChannelGroup 管理端：创建分组。
func CreateChannelGroup(c *gin.Context) {
	var payload channelGroupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	if payload.Name == "" {
		common.ApiErrorMsg(c, "分组名称不能为空")
		return
	}
	if payload.Name == model.DefaultChannelGroupName {
		common.ApiErrorMsg(c, "分组名称保留，禁止使用")
		return
	}
	if dup, err := model.IsChannelGroupNameDuplicated(0, payload.Name); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "分组名称已存在")
		return
	}
	group := payload.ChannelGroup
	group.Id = 0
	if err := group.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetChannelGroupChannels(group.Id, payload.ChannelIds); err != nil {
		common.ApiError(c, err)
		return
	}
	rebuildChannelGroupCaches()
	dto, err := buildChannelGroupDTO(&group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dto)
}

// UpdateChannelGroup 管理端：更新分组。默认分组只能改 profile/成员/描述，不能改名/删除/禁用。
func UpdateChannelGroup(c *gin.Context) {
	var payload channelGroupPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		common.ApiError(c, err)
		return
	}
	if payload.Id == 0 {
		common.ApiErrorMsg(c, "缺少分组 ID")
		return
	}
	existing, err := model.GetChannelGroupByID(payload.Id)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	group := payload.ChannelGroup
	if existing.IsDefault() {
		// 默认分组：强制保持名称与启用状态，仅允许改 profile/描述/成员。
		group.Name = existing.Name
		group.Enabled = true
	} else {
		if group.Name == "" {
			common.ApiErrorMsg(c, "分组名称不能为空")
			return
		}
		if group.Name == model.DefaultChannelGroupName {
			common.ApiErrorMsg(c, "分组名称保留，禁止使用")
			return
		}
		if dup, err := model.IsChannelGroupNameDuplicated(group.Id, group.Name); err != nil {
			common.ApiError(c, err)
			return
		} else if dup {
			common.ApiErrorMsg(c, "分组名称已存在")
			return
		}
	}
	if err := group.Update(); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.SetChannelGroupChannels(group.Id, payload.ChannelIds); err != nil {
		common.ApiError(c, err)
		return
	}
	rebuildChannelGroupCaches()
	dto, err := buildChannelGroupDTO(&group)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dto)
}

// DeleteChannelGroup 管理端：删除分组（默认分组拒绝）。
func DeleteChannelGroup(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DeleteChannelGroupByID(id); err != nil {
		common.ApiError(c, err)
		return
	}
	rebuildChannelGroupCaches()
	common.ApiSuccess(c, nil)
}

// GetAvailableChannelGroups 用户侧：返回可选分组的脱敏视图（id/name/description，绝不含渠道）。
func GetAvailableChannelGroups(c *gin.Context) {
	groups, err := model.GetAllChannelGroups()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	dtos := make([]availableChannelGroupDTO, 0, len(groups))
	for _, g := range groups {
		if !g.Enabled {
			continue
		}
		dtos = append(dtos, availableChannelGroupDTO{Id: g.Id, Name: g.Name, Description: g.Description})
	}
	common.ApiSuccess(c, dtos)
}

// rebuildChannelGroupCaches 在分组成员变化后重建 abilities 与渠道缓存。
// 分组成员属低频管理操作，全量重建简单且正确。
func rebuildChannelGroupCaches() {
	_, _, _ = model.FixAbility()
}

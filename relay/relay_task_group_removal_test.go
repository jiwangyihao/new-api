package relay

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskModel2DtoOmitsLegacyBusinessGroup(t *testing.T) {
	task := &model.Task{
		ID:        1,
		TaskID:    "task_legacy_group",
		UserId:    100,
		Group:     "vip",
		ChannelId: 200,
		Platform:  constant.TaskPlatform("video"),
		Status:    model.TaskStatusSuccess,
	}

	dtoTask := TaskModel2Dto(task)
	payload, err := common.Marshal(dtoTask)
	require.NoError(t, err)
	fields := map[string]interface{}{}
	require.NoError(t, common.Unmarshal(payload, &fields))
	for key := range fields {
		assert.False(t, strings.Contains(key, "group"), "unexpected group field %q in payload %s", key, string(payload))
	}
}

func TestInitTaskDoesNotPersistRelayUsingGroup(t *testing.T) {
	info := &relaycommon.RelayInfo{UserId: 101, UsingGroup: "vip", OriginModelName: "task-model", ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 201, ChannelType: 1}}
	task := model.InitTask(constant.TaskPlatform("video"), info)
	assert.Empty(t, task.Group)
}

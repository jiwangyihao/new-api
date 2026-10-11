# 渠道而外设置说明

该配置用于设置一些额外的渠道参数，可以通过 JSON 对象进行配置。主要包含以下两个设置项：

1. force_format
    - 用于标识是否对数据进行强制格式化为 OpenAI 格式
    - 类型为布尔值，设置为 true 时启用强制格式化

2. proxy
    - 用于配置网络代理
    - 类型为字符串，填写代理地址（例如 socks5 协议的代理地址）

3. thinking_to_content
   - 用于标识是否将思考内容`reasoning_content`转换为`<think>`标签拼接到内容中返回
   - 类型为布尔值，设置为 true 时启用思考内容转换

--------------------------------------------------------------

## JSON 格式示例

以下是一个示例配置，启用强制格式化并设置了代理地址：

```json
{
    "force_format": true,
   "thinking_to_content": true,
    "proxy": "socks5://xxxxxxx"
}
```

--------------------------------------------------------------

通过调整上述 JSON 配置中的值，可以灵活控制渠道的额外行为，比如是否进行格式化以及使用特定的网络代理。

## Responses 上游账号组

对接支持 `X-Account-Group` 的上游服务时，选择 **OpenAI** 渠道类型，在渠道编辑页的高级设置 → 字段透传控制 → **账号组** 中填写上游已配置的组名。该字段不是本项目的「渠道分组」，也不是 OpenAI Organization 或 Codex OAuth 的 `account_id`。

账号组保存在渠道 API 的 `settings` 字符串中（注意不是上文的 `setting`）：

```json
{
  "settings": "{\"account_group\":\"team-a\"}"
}
```

请在原有 `settings` JSON 对象中合并该字段，不要覆盖其他设置。普通 Responses 和 Responses Compact 请求会分别向上游 `/v1/responses`、`/v1/responses/compact` 发送：

```http
Authorization: Bearer <上游公共 API Key>
Content-Type: application/json
X-Account-Group: team-a
```

- 渠道密钥填写上游**公共 API Key**，不是租户管理面板或部署面板 Token。API 地址填写上游服务根地址。
- 去除组名前后的空白后发送；留空不自动添加该请求头，编辑时清空会移除保存的账号组限制。
- 仅 OpenAI 渠道的 Responses/Compact 生效（包括实际转换为 Responses 的请求）；普通 Chat Completions、其他端点和其他渠道类型不自动添加。无需修改模型请求 JSON。
- 仍遵循既有「请求头覆盖」优先级：同名显式覆盖、参数覆盖中的请求头操作或已配置的透传规则可以覆盖本设置。需要固定账号组时，不要配置会透传客户端 `X-Account-Group` 的通配规则。
- 对本次对接的上游，组名为 1–64 位小写字母、数字或连字符，首位为字母或数字；`default` 是保留名，不可填写。名称与账号归属由上游校验。
- 上游只在指定组内选账号；组不存在返回 400，无服务态账号返回 503。同时手动设置 `X-Account-Id` 时取交集，账号不属于该组返回 `400 account_group_mismatch`。本配置不会创建账号组或更改其成员。

协议来源：租户管理面板的 [API 端点目录](http://159.195.18.151/control/api-catalog) 中 `POST /v1/responses` 与 `POST /v1/responses/compact`。

package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/routehealth"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// These DTOs deliberately contain neither storage identities nor channel settings.
type RouteHealthReport struct {
	GeneratedAt          int64            `json:"generated_at"`
	Enabled              bool             `json:"enabled"`
	Storage              string           `json:"storage"`
	Degraded             bool             `json:"degraded"`
	RetryTimes           int              `json:"retry_times"`
	MaxAttempts          int              `json:"max_attempts"`
	SampleWindowSeconds  int              `json:"sample_window_seconds"`
	FailureWindowSeconds int              `json:"failure_window_seconds"`
	Rows                 []RouteHealthRow `json:"rows"`
}

type RouteHealthRow struct {
	ID               string                `json:"id"`
	ChannelID        int                   `json:"channel_id"`
	ChannelName      string                `json:"channel_name"`
	ChannelStatus    int                   `json:"channel_status"`
	Models           []string              `json:"models"`
	UpstreamModel    string                `json:"upstream_model"`
	Operation        string                `json:"operation"`
	State            string                `json:"state"`
	ReadyCredentials int                   `json:"ready_credentials"`
	TotalCredentials int                   `json:"total_credentials"`
	Resources        []RouteHealthResource `json:"resources"`
}

type RouteHealthResource struct {
	Scope               string `json:"scope"`
	CredentialIndex     int    `json:"credential_index"`
	State               string `json:"state"`
	Successes           uint64 `json:"successes"`
	Failures            uint64 `json:"failures"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	Inflight            int    `json:"inflight"`
	OpenedUntil         int64  `json:"opened_until"`
	NextProbe           int64  `json:"next_probe"`
	Probe               bool   `json:"probe"`
	RecoverySuccesses   int    `json:"recovery_successes"`
	Degraded            bool   `json:"degraded"`
}

type routeHealthOverviewEntry struct {
	row                RouteHealthRow
	resources          []routehealth.Resource
	indices            []int
	invalid            bool
	enabledCredentials int
}

// GetSetting/GetOtherSettings repair malformed JSON by saving the channel. Check
// first so this read-only endpoint never invokes those write-on-error branches.
func routeHealthConfigurationValid(channel *model.Channel) bool {
	var settings dto.ChannelSettings
	var other dto.ChannelOtherSettings
	return (channel.Setting == nil || *channel.Setting == "" || common.UnmarshalJsonStr(*channel.Setting, &settings) == nil) &&
		(channel.OtherSettings == "" || common.UnmarshalJsonStr(channel.OtherSettings, &other) == nil)
}

func routeHealthInventoryCredentials(channel *model.Channel) []model.ChannelCredential {
	// Disabled inventory still exposes its configured, individually enabled keys.
	// Work on a copy, never changing administrative status or the polling cursor.
	copy := *channel
	copy.Status = common.ChannelStatusEnabled
	credentials := copy.EnabledCredentials()
	result := make([]model.ChannelCredential, 0, len(credentials))
	seen := make(map[string]bool, len(credentials))
	for _, credential := range credentials {
		if strings.TrimSpace(credential.Key) == "" || seen[credential.Key] {
			continue
		}
		seen[credential.Key] = true
		result = append(result, credential)
	}
	return result
}

func routeHealthConfiguredCredentialCount(channel *model.Channel) int {
	if channel == nil || strings.TrimSpace(channel.Key) == "" {
		return 0
	}
	if !channel.ChannelInfo.IsMultiKey {
		return 1
	}
	keys := channel.GetKeys()
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	return len(seen)
}

type routeHealthOperation struct {
	name   string
	mapped bool
}

func routeHealthEndpointOperations(channel *model.Channel, modelName string, endpoint constant.EndpointType) []routeHealthOperation {
	var paths []string
	switch endpoint {
	case constant.EndpointTypeGemini:
		// The request URL.Path excludes query strings. Both /v1 and /v1beta
		// native paths are normalized by the admission operation function.
		for _, action := range []string{"generateContent", "streamGenerateContent", "embedContent", "batchEmbedContents", "predict"} {
			paths = append(paths, "/v1beta/models/"+modelName+":"+action)
		}
		paths = append(paths, "/v1/engines/"+modelName+"/embeddings")
	case constant.EndpointTypeOpenAI:
		paths = []string{"/v1/chat/completions", "/v1/completions", "/pg/chat/completions", "/v1/moderations", "/v1/realtime", "/v1/audio/speech", "/v1/audio/transcriptions", "/v1/audio/translations", "/v1/embeddings"}
	case constant.EndpointTypeImageGeneration:
		paths = []string{"/v1/images/generations", "/v1/images/edits", "/v1/edits"}
	case constant.EndpointTypeOpenAIVideo:
		paths = []string{"/v1/videos", "/v1/video/generations"}
	default:
		if info, ok := common.GetDefaultEndpointInfo(endpoint); ok {
			paths = []string{info.Path}
		} else {
			return []routeHealthOperation{{name: "endpoint:" + string(endpoint)}}
		}
	}
	operations := make([]routeHealthOperation, 0, len(paths)+1)
	for _, path := range paths {
		operations = append(operations, routeHealthOperation{name: routeOperation(nil, channel, modelName, path), mapped: true})
	}
	if endpoint == constant.EndpointTypeOpenAIResponse {
		apiType, supported := common.ChannelType2APIType(channel.Type)
		if channel.GetSetting().ResponsesWebSocketEnabled && supported && (apiType == constant.APITypeOpenAI || apiType == constant.APITypeCodex) {
			c := &gin.Context{Request: &http.Request{Method: http.MethodGet}}
			operations = append(operations, routeHealthOperation{name: routeOperation(c, channel, modelName, "/v1/responses"), mapped: true})
		}
	}
	return operations
}

func BuildRouteHealthOverview(ctx context.Context) (*RouteHealthReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	channels, err := model.GetAllChannels(0, 0, true, true)
	if err != nil {
		return nil, err
	}
	return buildRouteHealthOverview(ctx, channels, routeManager(), operation_setting.GetRouteHealthSetting().Policy(), time.Now(), common.RedisEnabled, common.RetryTimes)
}

func buildRouteHealthOverview(ctx context.Context, channels []*model.Channel, manager *routehealth.Manager, policy routehealth.Policy, now time.Time, redisConfigured bool, retries int) (*RouteHealthReport, error) {
	report := &RouteHealthReport{GeneratedAt: now.Unix(), Enabled: policy.Enabled, Storage: "memory", RetryTimes: retries, MaxAttempts: retries + 1,
		SampleWindowSeconds: int(policy.SampleWindow / time.Second), FailureWindowSeconds: int(policy.FailureWindow / time.Second), Rows: []RouteHealthRow{}}
	if redisConfigured {
		report.Storage = "redis"
	}
	entries := make([]routeHealthOverviewEntry, 0)
	rowIndex := make(map[string]int)
	resources := make([]routehealth.Resource, 0)
	seen := make(map[routehealth.Resource]bool)
	for _, channel := range channels {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if channel == nil {
			continue
		}
		valid := routeHealthConfigurationValid(channel)
		credentials := routeHealthInventoryCredentials(channel)
		totalCredentials := routeHealthConfiguredCredentialCount(channel)
		namespace := routeResources(channel)
		models := channel.GetModels()
		if len(models) == 0 {
			models = []string{""}
		}
		for _, alias := range models {
			mapped := ""
			invalid := !valid || strings.TrimSpace(alias) == ""
			if !invalid {
				var err error
				mapped, err = routeMappedModel(channel, alias)
				invalid = err != nil || strings.TrimSpace(mapped) == ""
			}
			operations := []routeHealthOperation{{name: "configuration"}}
			if !invalid {
				operations = nil
				for _, endpoint := range model.GetChannelEndpointTypes(channel, alias) {
					operations = append(operations, routeHealthEndpointOperations(channel, alias, endpoint)...)
				}
				if len(operations) == 0 {
					operations = []routeHealthOperation{{name: "configuration"}}
				}
			}
			for _, operation := range operations {
				// Length-delimited/public identity; no configuration/key fingerprint.
				identityModel := mapped
				if invalid {
					identityModel = alias
				}
				id := fmt.Sprintf("%d/%s/%s", channel.Id, url.PathEscape(identityModel), url.PathEscape(operation.name))
				if index, exists := rowIndex[id]; exists {
					entries[index].row.Models = append(entries[index].row.Models, alias)
					continue
				}
				entry := routeHealthOverviewEntry{row: RouteHealthRow{ID: id, ChannelID: channel.Id, ChannelName: channel.Name, ChannelStatus: channel.Status,
					Models: []string{alias}, UpstreamModel: mapped, Operation: operation.name, TotalCredentials: totalCredentials, Resources: []RouteHealthResource{}}, invalid: invalid || !operation.mapped, enabledCredentials: len(credentials)}
				entry.resources = append(entry.resources, namespace.channel())
				entry.indices = append(entry.indices, 0)
				if !entry.invalid {
					entry.resources = append(entry.resources, namespace.route(mapped, operation.name))
					entry.indices = append(entry.indices, 0)
				}
				for _, credential := range credentials {
					entry.resources = append(entry.resources, namespace.credential(credential.Key))
					entry.indices = append(entry.indices, credential.Index+1)
				}
				for _, resource := range entry.resources {
					if !seen[resource] {
						seen[resource] = true
						resources = append(resources, resource)
					}
				}
				rowIndex[id] = len(entries)
				entries = append(entries, entry)
			}
		}
	}
	snapshots, err := manager.InspectMany(ctx, resources, policy, now)
	if err != nil {
		return nil, err
	}
	byResource := make(map[routehealth.Resource]routehealth.Snapshot, len(snapshots))
	for _, snapshot := range snapshots {
		byResource[snapshot.Resource] = snapshot
		report.Degraded = report.Degraded || snapshot.Degraded
	}
	if redisConfigured && common.RDB == nil && policy.Enabled {
		report.Degraded = true
	}
	for _, entry := range entries {
		row := entry.row
		sort.Strings(row.Models)
		aliases := row.Models[:0]
		for _, alias := range row.Models {
			if len(aliases) == 0 || aliases[len(aliases)-1] != alias {
				aliases = append(aliases, alias)
			}
		}
		row.Models = aliases
		sharedReady := true
		sharedState := "unobserved"
		routeObserved := false
		credentialState := "cooling"
		credentialRank := -1
		for index, resource := range entry.resources {
			snapshot := byResource[resource]
			state := routeHealthDynamicState(snapshot, policy, now)
			row.Resources = append(row.Resources, RouteHealthResource{Scope: string(resource.Scope), CredentialIndex: entry.indices[index], State: state,
				Successes: snapshot.Successes, Failures: snapshot.Failures, ConsecutiveFailures: snapshot.ConsecutiveFailures, Inflight: snapshot.Inflight,
				OpenedUntil: routeHealthUnix(snapshot.OpenedUntil), NextProbe: routeHealthUnix(snapshot.NextProbe), Probe: snapshot.Probe, RecoverySuccesses: snapshot.RecoverySuccesses, Degraded: snapshot.Degraded})
			if resource.Scope != routehealth.ScopeCredential {
				sharedReady = sharedReady && routeHealthResourceReady(snapshot, policy, now)
				if routeHealthRestriction(state) > routeHealthRestriction(sharedState) {
					sharedState = state
				}
				if resource.Scope == routehealth.ScopeRoute {
					routeObserved = snapshot.Successes > 0 || snapshot.Failures > 0
				}
			} else {
				if routeHealthResourceReady(snapshot, policy, now) {
					row.ReadyCredentials++
				}
				// Prefer an ordinary admissible key over another key's recovery.
				rank := routeHealthCredentialRank(state)
				if rank > credentialRank {
					credentialState, credentialRank = state, rank
				}
			}
		}
		if !sharedReady {
			row.ReadyCredentials = 0
		}
		switch {
		case row.ChannelStatus == common.ChannelStatusManuallyDisabled:
			row.State = "manually_disabled"
		case row.ChannelStatus != common.ChannelStatusEnabled:
			row.State = "automatically_disabled"
		case entry.invalid:
			row.State = "invalid_mapping"
		case entry.enabledCredentials == 0:
			row.State = "no_credentials"
		case !policy.Enabled:
			row.State = "protection_disabled"
		default:
			row.State = sharedState
			if routeHealthRestriction(credentialState) > routeHealthRestriction(row.State) {
				row.State = credentialState
			}
			if routeHealthRestriction(row.State) == 0 {
				row.State = "unobserved"
				if routeObserved {
					row.State = "healthy"
				}
			}
		}
		if row.ChannelStatus != common.ChannelStatusEnabled || entry.invalid || row.TotalCredentials == 0 {
			row.ReadyCredentials = 0
		}
		report.Rows = append(report.Rows, row)
	}
	sort.Slice(report.Rows, func(i, j int) bool {
		a, b := report.Rows[i], report.Rows[j]
		if a.ChannelID != b.ChannelID {
			return a.ChannelID < b.ChannelID
		}
		if a.UpstreamModel != b.UpstreamModel {
			return a.UpstreamModel < b.UpstreamModel
		}
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		return a.ID < b.ID
	})
	return report, nil
}

func routeHealthUnix(at time.Time) int64 {
	if at.IsZero() {
		return 0
	}
	return at.Unix()
}

func routeHealthDynamicState(snapshot routehealth.Snapshot, policy routehealth.Policy, now time.Time) string {
	if !policy.Enabled {
		return "protection_disabled"
	}
	if snapshot.Open {
		if snapshot.OpenedUntil.After(now) {
			return "cooling"
		}
		return "recovering"
	}
	if snapshot.Inflight >= policy.MaxInflight {
		return "saturated"
	}
	if snapshot.Successes == 0 && snapshot.Failures == 0 {
		return "unobserved"
	}
	return "healthy"
}

func routeHealthResourceReady(snapshot routehealth.Snapshot, policy routehealth.Policy, now time.Time) bool {
	if !policy.Enabled {
		return true
	}
	return snapshot.Inflight < policy.MaxInflight && (!snapshot.Open || !snapshot.Probe && !snapshot.OpenedUntil.After(now) && !snapshot.NextProbe.After(now))
}

func routeHealthRestriction(state string) int {
	switch state {
	case "cooling":
		return 3
	case "recovering":
		return 2
	case "saturated":
		return 1
	default:
		return 0
	}
}

func routeHealthCredentialRank(state string) int {
	switch state {
	case "healthy", "unobserved", "protection_disabled":
		return 3
	case "recovering":
		return 2
	case "saturated":
		return 1
	default:
		return 0
	}
}

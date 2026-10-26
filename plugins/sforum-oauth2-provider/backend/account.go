package main

import (
	"context"
	"log"
	"strconv"
	"strings"
	"time"

	hostwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// 账号时间线（注册时间 / 资料更新时间）。
//
// 宿主的 core-safe 身份投影只有 8 个字段（id/username/displayName/status/
// avatar/groups/topicCount/commentCount），不含注册时间。注册时间来自宿主的稳定
// 查询 `sforum.core.safe_user.by_id`：它读 `sforum_core_v1.safe_users` 视图（公开
// 数据，注释原文就是 "active safe identity without email, credentials..."），
// 插件以 actorless 身份调用即可，不需要 actor。
//
// 代价是清单必须声明 `core_views` 授权（见 README「Host 约束」）：没有它宿主会返回
// `host.query_core_views_denied`。任何失败——旧宿主没有这条查询、授权未确认、用户
// 非 active 查不到行、字段类型不符——都降级成「字段缺席」，userinfo 的既有字段与
// 200 状态不受影响，绝不编造时间。
const (
	// accountQueryTimeout 与用户投影的 2s 预算对齐：userinfo 路由整体 5s。
	accountQueryTimeout = 2 * time.Second

	// safe_users 只投影 status='active' 的账号；被禁言/停用的用户查不到行。
	// 这不是错误，字段缺席即可（身份投影仍会给出 status）。
	accountQueryLimit = 1
)

// accountTimestamps 是 userinfo 暴露的账号时间线字段（UTC RFC3339Nano）。
type accountTimestamps struct {
	CreatedAt string
	UpdatedAt string
}

// loadAccountTimestamps 通过稳定查询读取账号的注册与更新时间。
// 返回的字符串是 UTC RFC3339Nano（宿主把 timestamptz 规范化成这个形式），
// 空串表示该字段未取到。
func (s *oauthServer) loadAccountTimestamps(
	ctx context.Context,
	hostCtx *protocolwire.RequestContext,
	userID int64,
) (accountTimestamps, bool) {
	if hostCtx == nil || userID <= 0 {
		return accountTimestamps{}, false
	}
	host, err := s.hostClient()
	if err != nil || host == nil || host.Queries == nil {
		// 旧宿主/单测注入的 Host 可能没有 Query 通道：静默缺席，不记为错误。
		return accountTimestamps{}, false
	}
	// 过滤器值必须带参数 schema：宿主按 (field, operator) 查白名单，并校验
	// `sforum.core.query.int64@1` 里的 value 是规范十进制字符串。
	filterValue, err := newDocument(queryInt64ParameterRef, map[string]any{
		"value": strconv.FormatInt(userID, 10),
	})
	if err != nil {
		s.recordLifecycleError("account query filter document: " + err.Error())
		return accountTimestamps{}, false
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, accountQueryTimeout)
	defer cancel()
	response, err := host.Queries.Execute(timeoutCtx, &hostwire.QueryRequest{
		Context:             hostCallContext(hostCtx),
		QueryId:             querySafeUserByID,
		PlanVersion:         queryStableCorePlanVersion,
		Fields:              []string{"id", "created_at", "updated_at"},
		Filters:             []*hostwire.QueryFilter{{Field: "id", Operator: "eq", Value: filterValue}},
		Page:                &protocolwire.PageRequest{Limit: accountQueryLimit},
		ResultSchemaId:      querySafeUserSchemaID,
		ResultSchemaVersion: querySafeUserSchemaVersion,
	})
	if err != nil || response == nil {
		s.recordLifecycleError(firstNonEmpty(errorText(err), "account query returned no response"))
		return accountTimestamps{}, false
	}
	if detail := response.GetError(); detail != nil {
		s.recordLifecycleError(detail.GetReason() + ": " + detail.GetMessage())
		return accountTimestamps{}, false
	}
	if len(response.GetRows()) == 0 {
		return accountTimestamps{}, false
	}
	values := response.GetRows()[0].GetValue().AsMap()
	timestamps := accountTimestamps{
		CreatedAt: normalizeTimestamp(values["created_at"]),
		UpdatedAt: normalizeTimestamp(values["updated_at"]),
	}
	if timestamps.CreatedAt == "" && timestamps.UpdatedAt == "" {
		return accountTimestamps{}, false
	}
	return timestamps, true
}

// normalizeTimestamp 只接受可解析的 RFC3339(Nano) 字符串，并统一成 UTC。
// 宿主理论上已经给这个形式；这里仍然校验，避免把 null/数字/时区怪值原样透给第三方。
func normalizeTimestamp(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
}

// recordLifecycleError 保存最近一次账号查询失败的原因，并在原因变化时打一条日志。
// 只用于运维定位，不进 userinfo 响应（内部错误不外泄）。相同原因连续出现只记一次，
// 避免稳定失败（例如授权未确认）把日志刷满。
func (s *oauthServer) recordLifecycleError(text string) {
	text = strings.TrimSpace(text)
	s.lifecycleErrorMu.Lock()
	previous := s.lifecycleError
	s.lifecycleError = text
	s.lifecycleErrorMu.Unlock()
	if text != "" && text != previous {
		log.Printf("oauth2-provider: account timestamps unavailable: %s", text)
	}
}

func (s *oauthServer) lastLifecycleError() string {
	s.lifecycleErrorMu.RLock()
	defer s.lifecycleErrorMu.RUnlock()
	return s.lifecycleError
}

package main

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	hostwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// settings 是插件运行期配置。默认值即 sforum.oauth2-provider 的推荐默认值，
// Host 设置不可用时仍然安全可跑。
type settings struct {
	AccessTokenTTL  int64
	RefreshTokenTTL int64
	CodeTTL         int64
	AllowUserApps   bool
	RequireConsent  bool
	AllowedScopes   []string
}

func defaultSettings() settings {
	return settings{
		AccessTokenTTL:  3600,
		RefreshTokenTTL: 2592000,
		CodeTTL:         600,
		AllowUserApps:   true,
		RequireConsent:  true,
		AllowedScopes: []string{
			scopeProfile,
			// email 敏感但可选：站点允许应用申请，应用必须显式请求、用户必须在确认页同意。
			scopeEmail,
			scopeAvatar,
			scopeTopicCount,
			scopeCommentCount,
			scopeUserGroups,
		},
	}
}

const (
	settingsCacheTTL   = 30 * time.Second
	hostQueryOwnSettID = "sforum.extensions.settings.own"
	hostQueryPlanV1    = "1"
	hostQueryResultID  = "sforum.extensions.settings.own.result"
	hostQueryResultV1  = "1"
)

// hostCallContext 构造插件 → Host 调用上下文。
// Host 的 validateProtocolV2HostContext 会拒绝任何携带 Actor 的插件请求
// （"protocol v2 actor context is not host-attested"），因此必须清空 Actor；
// 身份、授权与追踪信息保持不变。core 安全字段的读取不需要 actor。
func hostCallContext(routeCtx *protocolwire.RequestContext) *protocolwire.RequestContext {
	if routeCtx == nil {
		return nil
	}
	clone := proto.Clone(routeCtx).(*protocolwire.RequestContext)
	clone.Actor = nil
	// 宿主的委派是 Host-to-plugin 单向的：插件把 incoming context 原样回传会被判
	// "protocol v2 actor delegations are Host-to-plugin only"。委派令牌只能在对应的
	// delegated request 字段里提交（QueryRequest.actor_delegation 等，见 email_claim.go），
	// 所以这里连同会话委派一起清空。
	clone.HostCommandDelegations = nil
	clone.HostQueryDelegations = nil
	clone.HostSessionDelegation = nil
	return clone
}

// settingsSnapshot 返回缓存中的设置；过期或未加载时通过 Host 读一次。
func (s *oauthServer) settingsSnapshot(ctx context.Context, hostCtx *protocolwire.RequestContext) settings {
	s.settingsMu.RLock()
	cached := s.settingsCache
	fetchedAt := s.settingsFetchedAt
	s.settingsMu.RUnlock()
	if cached != nil && time.Since(fetchedAt) < settingsCacheTTL {
		return *cached
	}
	loaded, ok := s.loadSettings(ctx, hostCtx)
	if !ok {
		if cached != nil {
			return *cached
		}
		return defaultSettings()
	}
	s.settingsMu.Lock()
	s.settingsCache = &loaded
	s.settingsFetchedAt = time.Now()
	s.settingsMu.Unlock()
	return loaded
}

// settings 在不需要 Host 上下文的路径（令牌校验等）返回缓存或默认值。
func (s *oauthServer) settings() settings {
	s.settingsMu.RLock()
	cached := s.settingsCache
	s.settingsMu.RUnlock()
	if cached != nil {
		return *cached
	}
	return defaultSettings()
}

// loadSettings 通过 Host own-settings 查询读取本扩展设置。
func (s *oauthServer) loadSettings(ctx context.Context, hostCtx *protocolwire.RequestContext) (settings, bool) {
	result := defaultSettings()
	if hostCtx == nil {
		return result, false
	}
	host, err := s.hostClient()
	if err != nil || host == nil || host.Queries == nil {
		return result, false
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	response, err := host.Queries.Execute(timeoutCtx, &hostwire.QueryRequest{
		Context:             hostCallContext(hostCtx),
		QueryId:             hostQueryOwnSettID,
		PlanVersion:         hostQueryPlanV1,
		ResultSchemaId:      hostQueryResultID,
		ResultSchemaVersion: hostQueryResultV1,
		Fields: []string{
			settingAccessTTL, settingRefreshTTL, settingCodeTTL,
			settingUserApps, settingConsent, settingScopes,
		},
	})
	if err != nil || response == nil || response.GetError() != nil {
		return result, false
	}
	if len(response.GetRows()) == 0 {
		return result, false
	}
	values := response.GetRows()[0].GetValue().AsMap()
	container, _ := values["settings"].(map[string]any)
	if container == nil {
		return result, false
	}
	result.AccessTokenTTL = positiveInt(container[settingAccessTTL], result.AccessTokenTTL, 60, 86400)
	result.RefreshTokenTTL = positiveInt(container[settingRefreshTTL], result.RefreshTokenTTL, 300, 31536000)
	result.CodeTTL = positiveInt(container[settingCodeTTL], result.CodeTTL, 60, 3600)
	result.AllowUserApps = boolValue(container[settingUserApps], result.AllowUserApps)
	result.RequireConsent = boolValue(container[settingConsent], result.RequireConsent)
	if scopes := strings.Fields(stringValue(container[settingScopes])); len(scopes) > 0 {
		result.AllowedScopes = scopes
	}
	// profile 是身份读取的基底：始终允许、始终授予。管理员配置的清单只决定
	// 额外可选范围，不能把 profile 摘出去。
	if !slices.Contains(result.AllowedScopes, scopeProfile) {
		result.AllowedScopes = append([]string{scopeProfile}, result.AllowedScopes...)
	}
	return result, true
}

// lastProfileError 保存最近一次 Host 用户投影失败的原因，便于运营在
// userinfo 的错误描述里定位（Host 侧拒绝时不会给出更多细节）。
func (s *oauthServer) recordProfileError(text string) {
	s.profileErrorMu.Lock()
	s.profileError = text
	s.profileErrorMu.Unlock()
}

func (s *oauthServer) lastProfileError() string {
	s.profileErrorMu.RLock()
	defer s.profileErrorMu.RUnlock()
	return s.profileError
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func errorTextFromDetail(response *hostwire.IdentityUserResponse) string {
	if response == nil || response.GetError() == nil {
		return ""
	}
	detail := response.GetError()
	return detail.GetReason() + ": " + detail.GetMessage()
}

// userProfile 是 userinfo 端点对外暴露的资料。AvatarURL/Groups 来自宿主的
// core-safe 投影（公开数据）；TopicCount/CommentCount 由宿主按需查询公开内容
// 聚合，仅在 DeclaredFields 声明并成功返回时才有值（nil = 未取到，userinfo 省略）。
type userProfile struct {
	UserID       int64
	Username     string
	DisplayName  string
	Status       string
	AvatarURL    string
	Groups       []string
	TopicCount   *int64
	CommentCount *int64
}

// loadUserProfile 通过 Host IdentityService 读取 core-safe 用户字段。
// profile 与 avatar/groups 是核心安全投影（actorless 可读）；公开内容计数
// 由宿主在 DeclaredFields 显式声明时按需聚合。
func (s *oauthServer) loadUserProfile(
	ctx context.Context,
	hostCtx *protocolwire.RequestContext,
	userID int64,
) (userProfile, bool) {
	if hostCtx == nil || userID <= 0 {
		return userProfile{}, false
	}
	host, err := s.hostClient()
	if err != nil || host == nil || host.Identity == nil {
		return userProfile{}, false
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	response, err := host.Identity.GetUser(timeoutCtx, &hostwire.IdentityUserRequest{
		Context: hostCallContext(hostCtx),
		UserId:  userID,
		DeclaredFields: []string{
			"id", "username", "displayName", "status",
			"avatar", "groups", "topicCount", "commentCount",
		},
	})
	if err == nil && response != nil && response.GetError() == nil {
		return profileFromDocument(response, userID)
	}
	// 投影请求失败时退回不限定字段的调用：Host 会返回完整核心安全字段。
	fallbackCtx, fallbackCancel := context.WithTimeout(ctx, 2*time.Second)
	defer fallbackCancel()
	retry, retryErr := host.Identity.GetUser(fallbackCtx, &hostwire.IdentityUserRequest{
		Context: hostCallContext(hostCtx), UserId: userID,
	})
	if retryErr != nil || retry == nil || retry.GetError() != nil {
		s.recordProfileError(firstNonEmpty(errorText(err), errorText(retryErr),
			errorTextFromDetail(response), errorTextFromDetail(retry)))
		return userProfile{}, false
	}
	return profileFromDocument(retry, userID)
}

func profileFromDocument(response *hostwire.IdentityUserResponse, userID int64) (userProfile, bool) {
	values := response.GetUser().GetValue().AsMap()
	profile := userProfile{
		Username:    stringValue(values["username"]),
		DisplayName: stringValue(values["displayName"]),
		Status:      stringValue(values["status"]),
		AvatarURL:   stringValue(values["avatar"]),
	}
	if id, ok := values["id"].(float64); ok {
		profile.UserID = int64(id)
	} else {
		profile.UserID = userID
	}
	if rawGroups, ok := values["groups"].([]any); ok {
		groups := make([]string, 0, len(rawGroups))
		for _, item := range rawGroups {
			if name, ok := item.(string); ok && strings.TrimSpace(name) != "" {
				groups = append(groups, name)
			}
		}
		profile.Groups = groups
	}
	if count, ok := values["topicCount"].(float64); ok {
		value := int64(count)
		profile.TopicCount = &value
	}
	if count, ok := values["commentCount"].(float64); ok {
		value := int64(count)
		profile.CommentCount = &value
	}
	if profile.DisplayName == "" {
		profile.DisplayName = profile.Username
	}
	return profile, profile.Username != ""
}

func positiveInt(value any, fallback, minimum, maximum int64) int64 {
	parsed, ok := int64Value(value)
	if !ok || parsed < minimum || parsed > maximum {
		return fallback
	}
	return parsed
}

func int64Value(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func boolValue(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func stringValue(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

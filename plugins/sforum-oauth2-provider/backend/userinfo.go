package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
)

// 用户资料端点。访问令牌通过 access_token 查询参数交付：Host 的插件路由在
// filtered authority 模式下会剥离 Authorization 头，因此 OAuth2 的 Bearer 头
// 无法到达插件进程（详见 README 的限制说明）。
func (s *oauthServer) handleUserinfo(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	plaintext := firstNonEmpty(
		queryValue(request, "access_token"),
		bodyString(bodyValues(request), "access_token"),
	)
	// Host 会剥离凭据头，这里额外接受 x-access-token 便于调试脚本。
	if plaintext == "" {
		plaintext = queryValue(request, "token")
	}
	if plaintext == "" {
		return userinfoError(request, "invalid_request", "access_token is required")
	}
	record, ok := s.activeToken(ctx, plaintext, "access")
	if !ok {
		return userinfoError(request, "invalid_token", "the access token is invalid or has expired")
	}
	profile, ok := s.loadUserProfile(ctx, request.GetContext(), record.UserID)
	if !ok {
		reason := s.lastProfileError()
		if reason == "" {
			reason = "host identity lookup returned no core fields"
		}
		if len(reason) > 160 {
			reason = reason[:160]
		}
		return userinfoError(request, "server_error", "could not load the user profile: "+reason)
	}
	// profile 恒授予（必选）；可选字段只在该 scope 被授予时暴露。
	granted := map[string]struct{}{}
	for _, scope := range strings.Fields(record.Scope) {
		granted[scope] = struct{}{}
	}
	// id 与 sub 同值，但按 GitHub 的写法给数字形态：只认数字 id 的客户端
	// （把 sub 当字符串校验的那批）不用再自己转换。
	payload := map[string]any{
		"sub":         userSubject(profile.UserID),
		"id":          profile.UserID,
		"username":    profile.Username,
		"displayName": profile.DisplayName,
		"status":      profile.Status,
		"scope":       record.Scope,
		"clientId":    record.ClientID,
	}
	// 注册时间/资料更新时间来自宿主的稳定查询（core_views 授权）。查不到就整段
	// 省略：第三方据此区分「宿主没给」与「用户没有注册时间」，绝不编造。
	if timestamps, ok := s.loadAccountTimestamps(ctx, request.GetContext(), profile.UserID); ok {
		if timestamps.CreatedAt != "" {
			payload["createdAt"] = timestamps.CreatedAt
		}
		if timestamps.UpdatedAt != "" {
			payload["updatedAt"] = timestamps.UpdatedAt
		}
	}
	// profileUrl 是站内公开主页地址（派生值，不需要额外授权）。
	if profileURL := s.profileURL(ctx, profile.Username); profileURL != "" {
		payload["profileUrl"] = profileURL
	}
	// email 只读授权时的快照：userinfo 是服务端到服务端调用，拿不到宿主委派。
	// 没有快照（授权时读取失败、或老授权早于本功能）就整段省略，不回退成空串。
	if _, ok := granted[scopeEmail]; ok {
		if claim, found := s.store.getEmailClaim(ctx, record.UserID, record.ClientID); found && claim.Email != "" {
			payload["email"] = claim.Email
			payload["emailVerified"] = claim.EmailVerified
		}
	}
	// avatar 授予即下发：宿主有 URL 用它（相对路径补成绝对链接），没有
	// （initials 账号）就用确定性兜底头像。不能按「非空才写」处理——默认安装下
	// 所有账号都没有 URL，字段会整个消失，第三方无法区分「没授权」和「授权了但没有图」。
	if _, ok := granted[scopeAvatar]; ok {
		payload["avatar"] = s.avatarValueFor(ctx, profile)
	}
	if _, ok := granted[scopeUserGroups]; ok {
		payload["groups"] = groupNamesToAny(profile.Groups)
	}
	if _, ok := granted[scopeTopicCount]; ok && profile.TopicCount != nil {
		payload["topicCount"] = *profile.TopicCount
	}
	if _, ok := granted[scopeCommentCount]; ok && profile.CommentCount != nil {
		payload["commentCount"] = *profile.CommentCount
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaUserinfoResponse, payload)
}

func userinfoError(request *pluginwire.RouteRequest, code, description string) (*pluginwire.RouteResponse, error) {
	status := http.StatusBadRequest
	if code == "invalid_token" {
		status = http.StatusUnauthorized
	} else if code == "server_error" {
		status = http.StatusServiceUnavailable
	}
	document, err := newDocument(schemaUserinfoResponse, map[string]any{
		"error": code, "error_description": description,
	})
	if err != nil {
		return nil, err
	}
	return &pluginwire.RouteResponse{
		Context: responseContext(request.GetContext()), StatusCode: uint32(status),
		Headers: routeHeaders(jsonResponseHeaders), Body: document,
	}, nil
}

// groupNamesToAny 把用户组标识转成 structpb 可编码的 []any。
//
// 响应文档经 structpb.NewStruct 编码，protobuf 的 Value 只接受 []interface{}；
// 直接放 []string 会让整个响应编码失败（宿主侧同款坑是 identity_encode_failed），
// 表现为路由 502 extensions.route_unavailable。空列表也要是 [] 而不是 null。
func groupNamesToAny(names []string) []any {
	groups := make([]any, 0, len(names))
	for _, name := range names {
		groups = append(groups, name)
	}
	return groups
}

// userSubject 是 OIDC 风格的稳定用户标识（本站用户 id 的字符串形式）。
func userSubject(userID int64) string {
	return fmt.Sprintf("%d", userID)
}

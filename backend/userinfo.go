package main

import (
	"context"
	"fmt"
	"net/http"

	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
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
	return routeDocument(request.GetContext(), http.StatusOK, schemaUserinfoResponse, map[string]any{
		"sub":         userSubject(profile.UserID),
		"username":    profile.Username,
		"displayName": profile.DisplayName,
		"status":      profile.Status,
		"scope":       record.Scope,
		"clientId":    record.ClientID,
	})
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

// userSubject 是 OIDC 风格的稳定用户标识（本站用户 id 的字符串形式）。
func userSubject(userID int64) string {
	return fmt.Sprintf("%d", userID)
}

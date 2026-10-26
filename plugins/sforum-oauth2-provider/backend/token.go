package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
)

// 令牌端点：授权码兑换（含 PKCE 复核）与刷新令牌轮换。
// 客户端认证只接受请求体中的 client_id/client_secret：Host 以 filtered
// authority 交付请求，Authorization 头不会到达插件。

func (s *oauthServer) handleToken(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	values := bodyValues(request)
	grantType := bodyString(values, "grant_type")
	clientID := bodyString(values, "client_id")
	clientSecret := bodyString(values, "client_secret")

	client, err := s.authenticateClient(ctx, clientID, clientSecret)
	if err != nil {
		switch {
		case errors.Is(err, errClientNotFound):
			return tokenError(request, http.StatusUnauthorized, "invalid_client", "unknown client_id")
		default:
			return tokenError(request, http.StatusUnauthorized, "invalid_client", "client authentication failed")
		}
	}

	switch grantType {
	case "authorization_code":
		return s.exchangeAuthorizationCode(ctx, request, client, values)
	case "refresh_token":
		return s.exchangeRefreshToken(ctx, request, client, values)
	default:
		return tokenError(request, http.StatusBadRequest, "unsupported_grant_type",
			"supported grant types are authorization_code and refresh_token")
	}
}

func (s *oauthServer) exchangeAuthorizationCode(
	ctx context.Context,
	request *pluginwire.RouteRequest,
	client clientRecord,
	values map[string]any,
) (*pluginwire.RouteResponse, error) {
	code := bodyString(values, "code")
	if code == "" {
		return tokenError(request, http.StatusBadRequest, "invalid_request", "code is required")
	}
	record, ok := s.store.getToken(ctx, hashToken(code))
	if !ok || record.Kind != "code" || record.RevokedAt != "" {
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "authorization code is invalid")
	}
	if record.ClientID != client.ClientID {
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "authorization code was issued to another client")
	}
	if expiry, err := time.Parse(timeLayout, record.ExpiresAt); err != nil || !expiry.After(time.Now().UTC()) {
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "authorization code has expired")
	}
	if record.RedirectURI != "" {
		provided := bodyString(values, "redirect_uri")
		if provided != record.RedirectURI {
			return tokenError(request, http.StatusBadRequest, "invalid_grant", "redirect_uri does not match the authorization request")
		}
	}
	if err := verifyPKCE(record.CodeChallenge, record.CodeChallengeMethod, bodyString(values, "code_verifier")); err != nil {
		if errors.Is(err, errPKCEMissing) {
			return tokenError(request, http.StatusBadRequest, "invalid_request", "code_verifier is required")
		}
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "pkce verification failed")
	}
	// 授权码一次性使用：兑换前先作废，重复使用将被拒绝。
	if _, err := s.store.revokeToken(ctx, record.TokenHash); err != nil {
		return tokenError(request, http.StatusInternalServerError, "server_error", "could not consume the authorization code")
	}
	pair, err := s.issueTokens(ctx, client.ClientID, record.UserID, record.Scope, "", true)
	if err != nil {
		return tokenError(request, http.StatusInternalServerError, "server_error", "could not issue tokens")
	}
	s.store.touchClient(ctx, client.ClientID)
	return tokenSuccess(request, pair)
}

func (s *oauthServer) exchangeRefreshToken(
	ctx context.Context,
	request *pluginwire.RouteRequest,
	client clientRecord,
	values map[string]any,
) (*pluginwire.RouteResponse, error) {
	plaintext := bodyString(values, "refresh_token")
	if plaintext == "" {
		return tokenError(request, http.StatusBadRequest, "invalid_request", "refresh_token is required")
	}
	record, ok := s.store.getToken(ctx, hashToken(plaintext))
	if !ok || record.Kind != "refresh" || record.RevokedAt != "" {
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "refresh token is invalid")
	}
	if record.ClientID != client.ClientID {
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "refresh token belongs to another client")
	}
	if expiry, err := time.Parse(timeLayout, record.ExpiresAt); err != nil || !expiry.After(time.Now().UTC()) {
		return tokenError(request, http.StatusBadRequest, "invalid_grant", "refresh token has expired")
	}
	scope := record.Scope
	if requested := strings.TrimSpace(bodyString(values, "scope")); requested != "" {
		if !scopeSatisfies(record.Scope, requested) {
			return tokenError(request, http.StatusBadRequest, "invalid_scope", "requested scope exceeds the original grant")
		}
		scope = requested
	}
	// 轮换刷新令牌：旧令牌立即失效，缩小泄漏窗口。
	if _, err := s.store.revokeToken(ctx, record.TokenHash); err != nil {
		return tokenError(request, http.StatusInternalServerError, "server_error", "could not rotate the refresh token")
	}
	pair, err := s.issueTokens(ctx, client.ClientID, record.UserID, scope, record.TokenHash, true)
	if err != nil {
		return tokenError(request, http.StatusInternalServerError, "server_error", "could not issue tokens")
	}
	s.store.markRefreshUsed(ctx, record.TokenHash)
	s.store.touchClient(ctx, client.ClientID)
	return tokenSuccess(request, pair)
}

func tokenSuccess(request *pluginwire.RouteRequest, pair tokenPair) (*pluginwire.RouteResponse, error) {
	values := map[string]any{
		"access_token": pair.AccessToken,
		"token_type":   "Bearer",
		"expires_in":   pair.ExpiresIn,
		"scope":        pair.Scope,
	}
	if pair.RefreshToken != "" {
		values["refresh_token"] = pair.RefreshToken
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaTokenResponse, values)
}

func tokenError(request *pluginwire.RouteRequest, status int, code, description string) (*pluginwire.RouteResponse, error) {
	document, err := newDocument(schemaTokenResponse, map[string]any{
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

// handleRevoke 实现 RFC 7009 令牌撤销。撤销不存在的令牌同样返回成功。
func (s *oauthServer) handleRevoke(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	values := bodyValues(request)
	plaintext := bodyString(values, "token")
	if plaintext == "" {
		return routeDocument(request.GetContext(), http.StatusBadRequest, schemaRevokeResponse, map[string]any{
			"revoked": false, "error": "invalid_request", "errorDescription": "token is required",
		})
	}
	clientID := bodyString(values, "client_id")
	if clientID != "" {
		if _, err := s.authenticateClient(ctx, clientID, bodyString(values, "client_secret")); err != nil {
			return routeDocument(request.GetContext(), http.StatusUnauthorized, schemaRevokeResponse, map[string]any{
				"revoked": false, "error": "invalid_client", "errorDescription": "client authentication failed",
			})
		}
	}
	record, found := s.store.getToken(ctx, hashToken(plaintext))
	if !found {
		return routeDocument(request.GetContext(), http.StatusOK, schemaRevokeResponse, map[string]any{"revoked": false})
	}
	if clientID != "" && record.ClientID != clientID {
		return routeDocument(request.GetContext(), http.StatusOK, schemaRevokeResponse, map[string]any{"revoked": false})
	}
	revoked, err := s.store.revokeToken(ctx, record.TokenHash)
	if err != nil {
		return routeDocument(request.GetContext(), http.StatusInternalServerError, schemaRevokeResponse, map[string]any{
			"revoked": false, "error": "server_error", "errorDescription": "could not revoke the token",
		})
	}
	// 撤销刷新令牌时同时撤销它签发的访问令牌，避免留下可用凭据。
	if record.Kind == "refresh" {
		if _, err := s.store.revokeClientTokens(ctx, record.UserID, record.ClientID); err != nil {
			return routeDocument(request.GetContext(), http.StatusInternalServerError, schemaRevokeResponse, map[string]any{
				"revoked": false, "error": "server_error", "errorDescription": "could not revoke derived tokens",
			})
		}
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaRevokeResponse, map[string]any{"revoked": revoked})
}

// handleIntrospect 实现 RFC 7662 令牌内省。
func (s *oauthServer) handleIntrospect(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	values := bodyValues(request)
	plaintext := bodyString(values, "token")
	if plaintext == "" {
		return routeDocument(request.GetContext(), http.StatusBadRequest, schemaIntrospectResponse, map[string]any{
			"active": false, "error": "invalid_request", "errorDescription": "token is required",
		})
	}
	if clientID := bodyString(values, "client_id"); clientID != "" {
		if _, err := s.authenticateClient(ctx, clientID, bodyString(values, "client_secret")); err != nil {
			return routeDocument(request.GetContext(), http.StatusUnauthorized, schemaIntrospectResponse, map[string]any{
				"active": false, "error": "invalid_client", "errorDescription": "client authentication failed",
			})
		}
	}
	record, ok := s.store.getToken(ctx, hashToken(plaintext))
	if !ok || record.RevokedAt != "" || record.Kind == "code" {
		return routeDocument(request.GetContext(), http.StatusOK, schemaIntrospectResponse, map[string]any{"active": false})
	}
	expiry, err := time.Parse(timeLayout, record.ExpiresAt)
	if err != nil || !expiry.After(time.Now().UTC()) {
		return routeDocument(request.GetContext(), http.StatusOK, schemaIntrospectResponse, map[string]any{"active": false})
	}
	result := map[string]any{
		"active":     true,
		"scope":      record.Scope,
		"client_id":  record.ClientID,
		"token_type": tokenTypeLabel(record.Kind),
		"sub":        userSubject(record.UserID),
		"exp":        expiry.Unix(),
	}
	if profile, ok := s.loadUserProfile(ctx, request.GetContext(), record.UserID); ok {
		result["username"] = profile.Username
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaIntrospectResponse, result)
}

func tokenTypeLabel(kind string) string {
	if kind == "refresh" {
		return "refresh_token"
	}
	return "Bearer"
}

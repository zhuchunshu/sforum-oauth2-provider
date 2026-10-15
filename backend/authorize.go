package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// 授权端点：校验第三方请求 → 需要时跳转登录 / 授权确认 → 回跳第三方并带上授权码。

// oauthError 是 RFC 6749 §5.2 的错误语义。
type oauthError struct {
	Code        string
	Description string
	Status      int
}

func (e oauthError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Description)
}

func invalidRequest(description string) oauthError {
	return oauthError{Code: "invalid_request", Description: description, Status: http.StatusBadRequest}
}

func invalidScope(description string) oauthError {
	return oauthError{Code: "invalid_scope", Description: description, Status: http.StatusBadRequest}
}

func unauthorizedClient(description string) oauthError {
	return oauthError{Code: "invalid_client", Description: description, Status: http.StatusUnauthorized}
}

func unsupportedGrant(description string) oauthError {
	return oauthError{Code: "unsupported_grant_type", Description: description, Status: http.StatusBadRequest}
}

func invalidGrantError(description string) oauthError {
	return oauthError{Code: "invalid_grant", Description: description, Status: http.StatusBadRequest}
}

// redirectWithError 按 RFC 6749 §4.1.2.1 把错误回跳给第三方应用。
func redirectWithError(
	request *pluginwire.RouteRequest,
	redirectURI string,
	state string,
	code string,
	description string,
) (*pluginwire.RouteResponse, error) {
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("error", code)
	if description != "" {
		query.Set("error_description", description)
	}
	if state != "" {
		query.Set("state", state)
	}
	parsed.RawQuery = query.Encode()
	return routeRedirect(request.GetContext(), parsed.String())
}

// redirectWithCode 回跳第三方应用并携带授权码与 state。
func redirectWithCode(request *pluginwire.RouteRequest, redirectURI, state, code string) (*pluginwire.RouteResponse, error) {
	parsed, err := url.Parse(redirectURI)
	if err != nil {
		return nil, err
	}
	query := parsed.Query()
	query.Set("code", code)
	if state != "" {
		query.Set("state", state)
	}
	parsed.RawQuery = query.Encode()
	return routeRedirect(request.GetContext(), parsed.String())
}

// authorizeQuery 是授权端点与确认页共用的请求参数。
type authorizeQuery struct {
	ClientID            string
	RedirectURI         string
	ResponseType        string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

func parseAuthorizeQuery(values func(string) string) authorizeQuery {
	return authorizeQuery{
		ClientID:            strings.TrimSpace(values("client_id")),
		RedirectURI:         strings.TrimSpace(values("redirect_uri")),
		ResponseType:        strings.TrimSpace(values("response_type")),
		Scope:               strings.TrimSpace(values("scope")),
		State:               strings.TrimSpace(values("state")),
		CodeChallenge:       strings.TrimSpace(values("code_challenge")),
		CodeChallengeMethod: strings.TrimSpace(values("code_challenge_method")),
	}
}

// verifyAuthorizeRequest 完成除用户参与外的全部校验。
func (s *oauthServer) verifyAuthorizeRequest(
	ctx context.Context,
	query authorizeQuery,
	hostCtx *protocolwire.RequestContext,
) (clientRecord, string, error) {
	if query.ClientID == "" {
		return clientRecord{}, "", invalidRequest("client_id is required")
	}
	client, ok := s.store.getClient(ctx, query.ClientID)
	if !ok {
		return clientRecord{}, "", invalidRequest("unknown client_id")
	}
	if query.RedirectURI == "" {
		return clientRecord{}, "", invalidRequest("redirect_uri is required")
	}
	if !matchRedirectURI(client.RedirectURIs, query.RedirectURI) {
		return clientRecord{}, "", invalidRequest("redirect_uri is not registered for this client")
	}
	if query.ResponseType != "code" {
		return clientRecord{}, "", oauthError{
			Code: "unsupported_response_type", Description: "only response_type=code is supported",
			Status: http.StatusBadRequest,
		}
	}
	settings := s.settingsSnapshot(ctx, hostCtx)
	scopes, err := normalizeScopes(query.Scope, settings.AllowedScopes)
	if err != nil {
		return clientRecord{}, "", invalidScope("requested scope is not allowed")
	}
	if !validCodeChallenge(query.CodeChallenge, query.CodeChallengeMethod) {
		return clientRecord{}, "", invalidRequest("code_challenge is invalid")
	}
	if client.ClientType == "public" && query.CodeChallenge == "" {
		return clientRecord{}, "", invalidRequest("public clients must use PKCE (code_challenge)")
	}
	return client, joinScopes(scopes), nil
}

// handleAuthorize 处理 GET /api/oauth/authorize。
func (s *oauthServer) handleAuthorize(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	query := parseAuthorizeQuery(func(key string) string { return queryValue(request, key) })
	client, scope, err := s.verifyAuthorizeRequest(ctx, query, request.GetContext())
	if err != nil {
		var oauthErr oauthError
		if errors.As(err, &oauthErr) {
			if query.RedirectURI != "" && matchRedirectURI(client.RedirectURIs, query.RedirectURI) {
				return redirectWithError(request, query.RedirectURI, query.State, oauthErr.Code, oauthErr.Description)
			}
			return routeFailure(request.GetContext(), oauthErr.Status, "oauth2."+oauthErr.Code, oauthErr.Description), nil
		}
		return routeFailure(request.GetContext(), http.StatusBadRequest, "oauth2.invalid_request", err.Error()), nil
	}

	userID := actorUserID(request)
	if userID <= 0 {
		// 浏览器场景：先跳转站内登录页，登录后回到本端点。
		loginTarget := "/login?redirect=" + url.QueryEscape(request.GetPath())
		if rawQuery := rawQueryFromRequest(request); rawQuery != "" {
			loginTarget = "/login?redirect=" + url.QueryEscape(request.GetPath()+"?"+rawQuery)
		}
		return routeRedirect(request.GetContext(), loginTarget)
	}

	settings := s.settingsSnapshot(ctx, request.GetContext())
	consent, hasConsent := s.store.getConsent(ctx, userID, client.ClientID)
	// 首次授权是否要用户确认由 require_consent 决定；新增权限范围始终要确认。
	needsConsent := settings.RequireConsent
	if hasConsent {
		needsConsent = !scopeSatisfies(consent.Scope, scope)
	}
	if needsConsent {
		requestToken, err := generateToken(prefixConsentRequest)
		if err != nil {
			return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.token_generation_failed",
				"could not create a consent request"), nil
		}
		if err := s.store.putRequest(ctx, requestRecord{
			TokenHash: hashToken(requestToken), ClientID: client.ClientID, UserID: userID,
			RedirectURI: query.RedirectURI, Scope: scope, State: query.State,
			CodeChallenge: query.CodeChallenge, CodeChallengeMethod: query.CodeChallengeMethod,
			ExpiresAt: time.Now().UTC().Add(time.Duration(settings.CodeTTL) * time.Second).Format(timeLayout),
		}); err != nil {
			return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.consent_store_failed",
				"could not persist the consent request"), nil
		}
		return routeRedirect(request.GetContext(), "/api/oauth/consent?request="+url.QueryEscape(requestToken))
	}

	if !hasConsent {
		// require_consent 关闭时的首次授权：直接放行，但仍记录授权以便用户管理。
		_ = s.store.upsertConsent(ctx, userID, client.ClientID, scope)
	}
	code, err := s.issueAuthorizationCode(ctx, client, userID, scope, query)
	if err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.code_generation_failed",
			"could not issue an authorization code"), nil
	}
	return redirectWithCode(request, query.RedirectURI, query.State, code)
}

// rawQueryFromRequest 还原 Host 传入的查询串，用于登录后回到同一个授权请求。
func rawQueryFromRequest(request *pluginwire.RouteRequest) string {
	values := url.Values{}
	for key, value := range request.GetQueryParameters() {
		if key == "" {
			continue
		}
		values.Set(key, value)
	}
	return values.Encode()
}

// issueAuthorizationCode 签发一次性授权码（kind=code）。
func (s *oauthServer) issueAuthorizationCode(
	ctx context.Context,
	client clientRecord,
	userID int64,
	scope string,
	query authorizeQuery,
) (string, error) {
	settings := s.settings()
	code, err := generateToken(prefixAuthorizationCode)
	if err != nil {
		return "", err
	}
	record := tokenRecord{
		TokenHash: hashToken(code), Kind: "code", ClientID: client.ClientID, UserID: userID,
		Scope: scope, RedirectURI: query.RedirectURI,
		CodeChallenge: query.CodeChallenge, CodeChallengeMethod: query.CodeChallengeMethod,
		ExpiresAt: time.Now().UTC().Add(time.Duration(settings.CodeTTL) * time.Second).Format(timeLayout),
	}
	if err := s.store.putToken(ctx, record); err != nil {
		return "", err
	}
	return code, nil
}

// handleConsentSummary 给确认页提供展示数据（GET /api/oauth/consent/summary）。
func (s *oauthServer) handleConsentSummary(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	userID := actorUserID(request)
	requestToken := queryValue(request, "request")
	pending, ok := s.store.getRequest(ctx, hashToken(requestToken))
	if !ok || pending.UserID != userID {
		return routeDocument(request.GetContext(), http.StatusOK, schemaConsentSummaryResponse, map[string]any{
			"status":     "expired",
			"clientName": "",
			"scopes":     []any{},
			"user":       map[string]any{"id": userID, "username": "", "displayName": ""},
			"error":      "consent request expired or belongs to another user",
		})
	}
	if expiry, err := time.Parse(timeLayout, pending.ExpiresAt); err != nil || !expiry.After(time.Now().UTC()) {
		s.store.deleteRequest(ctx, pending.TokenHash)
		return routeDocument(request.GetContext(), http.StatusOK, schemaConsentSummaryResponse, map[string]any{
			"status":     "expired",
			"clientName": "",
			"scopes":     []any{},
			"user":       map[string]any{"id": userID, "username": "", "displayName": ""},
			"error":      "consent request expired",
		})
	}
	client, found := s.store.getClient(ctx, pending.ClientID)
	clientName := pending.ClientID
	description := ""
	if found {
		clientName = client.Name
		description = client.Description
	}
	scopes := make([]any, 0)
	for _, scope := range strings.Fields(pending.Scope) {
		scopes = append(scopes, map[string]any{"key": scope, "label": scopeLabel(scope)})
	}
	profile, ok := s.loadUserProfile(ctx, request.GetContext(), userID)
	user := map[string]any{"id": userID, "username": "", "displayName": ""}
	if ok {
		user = map[string]any{"id": profile.UserID, "username": profile.Username, "displayName": profile.DisplayName}
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaConsentSummaryResponse, map[string]any{
		"status":            "ok",
		"clientId":          pending.ClientID,
		"clientName":        clientName,
		"clientDescription": description,
		"redirectUri":       pending.RedirectURI,
		"scopes":            scopes,
		"user":              user,
	})
}

// scopeLabel 提供人类可读的权限说明。
func scopeLabel(scope string) string {
	switch scope {
	case scopeProfile:
		return "读取你的用户名、昵称与账号状态"
	default:
		return scope
	}
}

// handleConsentDecision 处理确认页的允许/拒绝（POST /api/oauth/consent/decision）。
func (s *oauthServer) handleConsentDecision(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	values := bodyValues(request)
	userID := actorUserID(request)
	requestToken := bodyString(values, "request")
	decision := bodyString(values, "decision")
	pending, ok := s.store.getRequest(ctx, hashToken(requestToken))
	if !ok || pending.UserID != userID {
		return routeDocument(request.GetContext(), http.StatusBadRequest, schemaConsentDecisionResponse, map[string]any{
			"status": "error", "error": "invalid_request",
			"errorDescription": "consent request is unknown or expired",
		})
	}
	s.store.deleteRequest(ctx, pending.TokenHash)
	if decision != "allow" {
		parsed, err := url.Parse(pending.RedirectURI)
		if err != nil {
			return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
				"status": "error", "error": "invalid_request", "errorDescription": "invalid redirect_uri",
			})
		}
		query := parsed.Query()
		query.Set("error", "access_denied")
		query.Set("error_description", "the user denied the request")
		if pending.State != "" {
			query.Set("state", pending.State)
		}
		parsed.RawQuery = query.Encode()
		return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
			"status": "redirect", "location": parsed.String(),
		})
	}
	if bodyBool(values, "remember") {
		if err := s.store.upsertConsent(ctx, userID, pending.ClientID, pending.Scope); err != nil {
			return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
				"status": "error", "error": "server_error", "errorDescription": "could not store consent",
			})
		}
	}
	client, found := s.store.getClient(ctx, pending.ClientID)
	if !found {
		return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
			"status": "error", "error": "invalid_request", "errorDescription": "unknown client",
		})
	}
	code, err := s.issueAuthorizationCode(ctx, client, userID, pending.Scope, authorizeQuery{
		ClientID: pending.ClientID, RedirectURI: pending.RedirectURI, State: pending.State,
		CodeChallenge: pending.CodeChallenge, CodeChallengeMethod: pending.CodeChallengeMethod,
	})
	if err != nil {
		return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
			"status": "error", "error": "server_error", "errorDescription": "could not issue an authorization code",
		})
	}
	parsed, err := url.Parse(pending.RedirectURI)
	if err != nil {
		return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
			"status": "error", "error": "invalid_request", "errorDescription": "invalid redirect_uri",
		})
	}
	query := parsed.Query()
	query.Set("code", code)
	if pending.State != "" {
		query.Set("state", pending.State)
	}
	parsed.RawQuery = query.Encode()
	return routeDocument(request.GetContext(), http.StatusOK, schemaConsentDecisionResponse, map[string]any{
		"status": "redirect", "location": parsed.String(),
	})
}

// handleRouteStream 服务授权确认页：stream 模式先 preflight，再一次性输出 HTML。
// Host 在 Open().Path 上带上原始查询串，request 令牌从那里读取。
func (s *oauthServer) handleRouteStream(stream *pluginv2.RouteStream) error {
	if stream == nil || stream.Open() == nil {
		return fmt.Errorf("missing route stream open")
	}
	open := stream.Open()
	if open.GetRouteId() != routeConsentPageID {
		return stream.Close(&pluginwire.RouteStreamClose{
			StatusCode: http.StatusNotFound,
			Error: &protocolwire.ErrorDetail{
				Code: protocolwire.ErrorCode_ERROR_CODE_NOT_FOUND,
				Reason: "oauth2.stream_route_unknown", Message: "unknown oauth2-provider stream route",
			},
		})
	}
	requestToken := ""
	if parsed, err := url.Parse(open.GetPath()); err == nil {
		requestToken = parsed.Query().Get("request")
	}
	payload := []byte(renderConsentPage(requestToken, s.fallbackAuthorizeQuery(open)))
	if err := stream.Send(&protocolwire.DataChunk{
		Sequence: 1, Data: payload, Final: true,
	}); err != nil {
		return err
	}
	// 确认页是纯响应流：发送完毕立即关闭，不等待请求方向的空帧。
	return stream.Close(&pluginwire.RouteStreamClose{StatusCode: http.StatusOK})
}

// fallbackAuthorizeQuery 在确认页无法查询到待确认请求时，提供返回授权端点的链接。
func (s *oauthServer) fallbackAuthorizeQuery(open *pluginwire.RouteStreamOpen) string {
	if parsed, err := url.Parse(open.GetPath()); err == nil {
		if redirect := parsed.Query().Get("return"); redirect != "" {
			return redirect
		}
	}
	return "/api/oauth/authorize"
}

package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"

	pluginv2 "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

func testRequest(routeID string, values map[string]any) *pluginwire.RouteRequest {
	document, err := pluginv2.NewTypedDocument("sforum.oauth2-provider.test@1", values)
	if err != nil {
		panic(err)
	}
	return &pluginwire.RouteRequest{
		RouteId: routeID,
		Context: &protocolwire.RequestContext{
			RequestId: "test-request",
			Extension: &protocolwire.ExtensionIdentity{ExtensionId: "sforum.oauth2-provider"},
			// 默认 actor 是「持有 create_clients 的普通用户」：应用管理类用例以此为基线，
			// 无权限/管理员等差异场景在用例内显式覆盖 Actor。
			Actor: &protocolwire.Actor{UserId: 7, PermissionKeys: []string{permissionCreateClients}},
		},
		Method: "POST",
		Path:   "/api/oauth/token",
		Body:   document,
	}
}

// testServer 构造只依赖内存存储的服务器实例，不接触 Host 设置与用户投影。
func testServer(t *testing.T) *oauthServer {
	t.Helper()
	server, err := newOAuthServer()
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	if server.store.db != nil {
		t.Fatalf("memory store expected in unit tests")
	}
	cached := defaultSettings()
	server.settingsCache = &cached
	return server
}

func seedClient(t *testing.T, server *oauthServer, clientType, secret string) clientRecord {
	t.Helper()
	secretHash := ""
	if secret != "" {
		secretHash = hashToken(secret)
	}
	record := clientRecord{
		ClientID: "sfo_client_test0001", SecretHash: secretHash, Name: "Test App",
		ClientType: clientType, RedirectURIs: []string{"https://app.example.com/callback"},
		Scopes: []string{scopeProfile}, OwnerUserID: 7, CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := server.store.putClient(context.Background(), record); err != nil {
		t.Fatalf("put client: %v", err)
	}
	return record
}

// errorReason 读取错误响应里的 reason。错误现在以终端响应（StatusCode + 符合
// responseSchema 的 Body）返回，宿主不再接受只带 Error 字段的响应（会被映射成
// 502 extensions.route_unavailable），所以这里从 body 反推 reason。
func errorReason(t *testing.T, response *pluginwire.RouteResponse) string {
	t.Helper()
	if response == nil {
		return ""
	}
	if detail := response.GetError(); detail != nil {
		return detail.GetReason()
	}
	if response.GetBody() == nil || response.GetBody().GetValue() == nil {
		return ""
	}
	values := response.GetBody().GetValue().AsMap()
	if code, ok := values["error"].(string); ok && code != "" {
		return "oauth2." + code
	}
	if status, ok := values["status"].(string); ok && status == "error" {
		if code, ok := values["error"].(string); ok {
			return "oauth2." + code
		}
	}
	return ""
}

// hasError 判断响应是否为错误响应（Error 字段或 status=error / 带 error 键）。
func hasError(response *pluginwire.RouteResponse) bool {
	if response == nil {
		return false
	}
	if response.GetError() != nil {
		return true
	}
	if response.GetBody() == nil || response.GetBody().GetValue() == nil {
		return false
	}
	values := response.GetBody().GetValue().AsMap()
	if _, ok := values["error"].(string); ok {
		return true
	}
	status, _ := values["status"].(string)
	return status == "error"
}

func responseValues(t *testing.T, response *pluginwire.RouteResponse) map[string]any {
	t.Helper()
	if response == nil || response.GetBody() == nil || response.GetBody().GetValue() == nil {
		t.Fatalf("response has no body: %#v", response)
	}
	return response.GetBody().GetValue().AsMap()
}

func TestNormalizeScopes(t *testing.T) {
	scopes, err := normalizeScopes("profile profile", []string{scopeProfile})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 1 || scopes[0] != scopeProfile {
		t.Fatalf("scopes = %#v", scopes)
	}
	if _, err := normalizeScopes("email", []string{scopeProfile}); !errors.Is(err, errUnknownScope) {
		t.Fatalf("unknown scope must be rejected, got %v", err)
	}
	// profile 必选：请求了其他 scope 但没带 profile 时，后端无条件并入且排在最前。
	scopes, err = normalizeScopes("topic.count users.groups", []string{scopeProfile, scopeTopicCount, scopeUserGroups})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(scopes) != 3 || scopes[0] != scopeProfile || scopes[1] != scopeTopicCount || scopes[2] != scopeUserGroups {
		t.Fatalf("profile union failed: %#v", scopes)
	}
	scopes, err = normalizeScopes("", nil)
	if err != nil || len(scopes) != 1 || scopes[0] != scopeProfile {
		t.Fatalf("empty scope must default to profile: %#v %v", scopes, err)
	}
}

func TestScopeSatisfies(t *testing.T) {
	if !scopeSatisfies("profile", "profile") {
		t.Fatal("identical scopes must satisfy")
	}
	if scopeSatisfies("profile", "profile email") {
		t.Fatal("missing scope must not satisfy")
	}
	if !scopeSatisfies("profile email", "profile") {
		t.Fatal("subset must satisfy")
	}
}

func TestMatchRedirectURIRules(t *testing.T) {
	registered := []string{"https://app.example.com/callback"}
	if !matchRedirectURI(registered, "https://app.example.com/callback") {
		t.Fatal("exact match must pass")
	}
	if matchRedirectURI(registered, "https://app.example.com/callback/extra") {
		t.Fatal("prefix must not match")
	}
	if matchRedirectURI(registered, "https://evil.example.com/callback") {
		t.Fatal("other host must not match")
	}
	if matchRedirectURI(registered, "https://app.example.com/callback#frag") {
		t.Fatal("fragment must be rejected")
	}
	if matchRedirectURI(registered, "http://app.example.com/callback") {
		t.Fatal("plain http off loopback must be rejected")
	}
	if !validRedirectURIRegistration("http://127.0.0.1:8000/callback") {
		t.Fatal("loopback http must be allowed for local development")
	}
}

func TestVerifyPKCE(t *testing.T) {
	verifier := strings.Repeat("a", 43)
	digest := sha256.Sum256([]byte(verifier))
	// RFC 7636：S256 challenge 是 base64url(sha256(verifier))，无填充。
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])
	if err := verifyPKCE(challenge, "S256", verifier); err != nil {
		t.Fatalf("S256 verification failed: %v", err)
	}
	if err := verifyPKCE(challenge, "S256", strings.Repeat("b", 43)); !errors.Is(err, errPKCEFailed) {
		t.Fatalf("wrong verifier must fail, got %v", err)
	}
	if err := verifyPKCE(challenge, "S256", ""); !errors.Is(err, errPKCEMissing) {
		t.Fatalf("missing verifier must be reported, got %v", err)
	}
	if err := verifyPKCE("", "", ""); err != nil {
		t.Fatalf("no PKCE must be allowed: %v", err)
	}
	// 回归：十六进制编码的 challenge 不是 RFC 7636 形式，必须拒绝。
	if err := verifyPKCE(hex.EncodeToString(digest[:]), "S256", verifier); !errors.Is(err, errPKCEFailed) {
		t.Fatalf("hex-encoded challenge must be rejected, got %v", err)
	}
	// plain 方式按原样比较。
	if err := verifyPKCE(verifier, "plain", verifier); err != nil {
		t.Fatalf("plain verification failed: %v", err)
	}
	if !validCodeChallenge(challenge, "S256") {
		t.Fatal("valid challenge must pass")
	}
	if validCodeChallenge(challenge, "MD5") {
		t.Fatal("unsupported method must fail")
	}
	if validCodeChallenge(verifier, "") {
		t.Fatal("challenge without method must fail")
	}
}

func TestAuthenticateClient(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	if _, err := server.authenticateClient(ctx, "sfo_client_test0001", "sfos_secret_value"); err != nil {
		t.Fatalf("valid secret rejected: %v", err)
	}
	if _, err := server.authenticateClient(ctx, "sfo_client_test0001", "sfos_wrong"); !errors.Is(err, errInvalidSecret) {
		t.Fatalf("wrong secret must fail, got %v", err)
	}
	if _, err := server.authenticateClient(ctx, "sfo_client_missing", "x"); !errors.Is(err, errClientNotFound) {
		t.Fatalf("unknown client must fail, got %v", err)
	}
}

func TestPublicClientRequiresPKCE(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "public", "")
	ctx := context.Background()
	_, _, err := server.verifyAuthorizeRequest(ctx, authorizeQuery{
		ClientID: "sfo_client_test0001", RedirectURI: "https://app.example.com/callback",
		ResponseType: "code",
	}, nil)
	var oauthErr oauthError
	if !errors.As(err, &oauthErr) || oauthErr.Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %v", err)
	}
	if _, _, err := server.verifyAuthorizeRequest(ctx, authorizeQuery{
		ClientID: "sfo_client_test0001", RedirectURI: "https://app.example.com/callback",
		ResponseType: "code", CodeChallenge: strings.Repeat("c", 43), CodeChallengeMethod: "S256",
	}, nil); err != nil {
		t.Fatalf("PKCE public client must pass: %v", err)
	}
}

func TestAuthorizationCodeExchangeAndRotation(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	verifier := strings.Repeat("v", 43)
	digest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(digest[:])

	client, ok := server.store.getClient(ctx, "sfo_client_test0001")
	if !ok {
		t.Fatal("client missing")
	}
	code, err := server.issueAuthorizationCode(ctx, client, 7, scopeProfile, authorizeQuery{
		ClientID: client.ClientID, RedirectURI: "https://app.example.com/callback",
		CodeChallenge: challenge, CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}

	exchange := testRequest(routeTokenID, map[string]any{
		"grant_type": "authorization_code", "client_id": client.ClientID,
		"client_secret": "sfos_secret_value", "code": code,
		"redirect_uri": "https://app.example.com/callback", "code_verifier": verifier,
	})
	response, err := server.handleToken(ctx, exchange)
	if err != nil {
		t.Fatalf("exchange failed: %v", err)
	}
	if response.GetStatusCode() != uint32(http.StatusOK) {
		t.Fatalf("status = %d body=%v", response.GetStatusCode(), responseValues(t, response))
	}
	values := responseValues(t, response)
	accessToken, _ := values["access_token"].(string)
	refreshToken, _ := values["refresh_token"].(string)
	if !strings.HasPrefix(accessToken, prefixAccessToken) || !strings.HasPrefix(refreshToken, prefixRefreshToken) {
		t.Fatalf("unexpected token shapes: %v", values)
	}
	if _, ok := server.activeToken(ctx, accessToken, "access"); !ok {
		t.Fatal("issued access token must be active")
	}

	// 授权码一次性使用：重放必须失败。
	replay, err := server.handleToken(ctx, testRequest(routeTokenID, map[string]any{
		"grant_type": "authorization_code", "client_id": client.ClientID,
		"client_secret": "sfos_secret_value", "code": code,
		"redirect_uri": "https://app.example.com/callback", "code_verifier": verifier,
	}))
	if err != nil {
		t.Fatalf("replay call failed: %v", err)
	}
	if replay.GetStatusCode() != uint32(http.StatusBadRequest) {
		t.Fatalf("replayed code must be rejected, got %d", replay.GetStatusCode())
	}
	if code, _ := responseValues(t, replay)["error"].(string); code != "invalid_grant" {
		t.Fatalf("replay error = %v", responseValues(t, replay))
	}

	// 刷新令牌轮换：旧刷新令牌立即失效，新令牌可用。
	refreshed, err := server.handleToken(ctx, testRequest(routeTokenID, map[string]any{
		"grant_type": "refresh_token", "client_id": client.ClientID,
		"client_secret": "sfos_secret_value", "refresh_token": refreshToken,
	}))
	if err != nil {
		t.Fatalf("refresh failed: %v", err)
	}
	if refreshed.GetStatusCode() != uint32(http.StatusOK) {
		t.Fatalf("refresh status = %d body=%v", refreshed.GetStatusCode(), responseValues(t, refreshed))
	}
	if _, err := server.handleToken(ctx, testRequest(routeTokenID, map[string]any{
		"grant_type": "refresh_token", "client_id": client.ClientID,
		"client_secret": "sfos_secret_value", "refresh_token": refreshToken,
	})); err != nil {
		t.Fatalf("second refresh call failed: %v", err)
	}
	rotated := responseValues(t, refreshed)
	newRefresh, _ := rotated["refresh_token"].(string)
	if newRefresh == refreshToken {
		t.Fatal("refresh token must be rotated")
	}
}

func TestTokenExchangeRejectsWrongPKCEAndSecret(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	client, _ := server.store.getClient(ctx, "sfo_client_test0001")
	code, err := server.issueAuthorizationCode(ctx, client, 7, scopeProfile, authorizeQuery{
		ClientID: client.ClientID, RedirectURI: "https://app.example.com/callback",
		CodeChallenge: strings.Repeat("d", 43), CodeChallengeMethod: "S256",
	})
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	response, err := server.handleToken(ctx, testRequest(routeTokenID, map[string]any{
		"grant_type": "authorization_code", "client_id": client.ClientID,
		"client_secret": "sfos_wrong", "code": code,
		"redirect_uri": "https://app.example.com/callback", "code_verifier": strings.Repeat("e", 43),
	}))
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if response.GetStatusCode() != uint32(http.StatusUnauthorized) {
		t.Fatalf("wrong secret must fail with 401, got %d", response.GetStatusCode())
	}
	if code, _ := responseValues(t, response)["error"].(string); code != "invalid_client" {
		t.Fatalf("error = %v", responseValues(t, response))
	}
}

func TestRevokeInvalidatesAccessToken(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	client, _ := server.store.getClient(ctx, "sfo_client_test0001")
	pair, err := server.issueTokens(ctx, client.ClientID, 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	response, err := server.handleRevoke(ctx, testRequest(routeRevokeID, map[string]any{
		"token": pair.AccessToken, "client_id": client.ClientID, "client_secret": "sfos_secret_value",
	}))
	if err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if revoked, _ := responseValues(t, response)["revoked"].(bool); !revoked {
		t.Fatalf("revoke response = %v", responseValues(t, response))
	}
	if _, ok := server.activeToken(ctx, pair.AccessToken, "access"); ok {
		t.Fatal("revoked access token must not be active")
	}
}

func TestIntrospectReportsInactiveForRevoked(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	client, _ := server.store.getClient(ctx, "sfo_client_test0001")
	pair, err := server.issueTokens(ctx, client.ClientID, 7, scopeProfile, "", false)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	response, err := server.handleIntrospect(ctx, testRequest(routeIntrospectID, map[string]any{
		"token": pair.AccessToken, "client_id": client.ClientID, "client_secret": "sfos_secret_value",
	}))
	if err != nil {
		t.Fatalf("introspect failed: %v", err)
	}
	if active, _ := responseValues(t, response)["active"].(bool); !active {
		t.Fatalf("active token must introspect as active: %v", responseValues(t, response))
	}
	if _, err := server.store.revokeToken(ctx, pair.AccessHash); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	response, err = server.handleIntrospect(ctx, testRequest(routeIntrospectID, map[string]any{
		"token": pair.AccessToken, "client_id": client.ClientID, "client_secret": "sfos_secret_value",
	}))
	if err != nil {
		t.Fatalf("introspect after revoke failed: %v", err)
	}
	if active, _ := responseValues(t, response)["active"].(bool); active {
		t.Fatal("revoked token must introspect as inactive")
	}
}

func TestUserinfoRequiresValidAccessToken(t *testing.T) {
	server := testServer(t)
	request := testRequest(routeUserinfoID, map[string]any{})
	request.QueryParameters = map[string]string{}
	response, err := server.handleUserinfo(context.Background(), request)
	if err != nil {
		t.Fatalf("userinfo failed: %v", err)
	}
	if response.GetStatusCode() != uint32(http.StatusBadRequest) {
		t.Fatalf("missing token must be 400, got %d", response.GetStatusCode())
	}
	request.QueryParameters = map[string]string{"access_token": "sfoa_unknown"}
	response, err = server.handleUserinfo(context.Background(), request)
	if err != nil {
		t.Fatalf("userinfo failed: %v", err)
	}
	if response.GetStatusCode() != uint32(http.StatusUnauthorized) {
		t.Fatalf("invalid token must be 401, got %d", response.GetStatusCode())
	}
}

func TestClientManagementPermissions(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()

	// 非所有者不能删除他人应用。
	foreign := testRequest(routeClientsDeleteID, map[string]any{"clientId": "sfo_client_test0001"})
	foreign.Context.Actor = &protocolwire.Actor{UserId: 99}
	response, err := server.handleClientDelete(ctx, foreign)
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if !hasError(response) {
		t.Fatal("foreign owner deletion must fail closed")
	}
	if _, ok := server.store.getClient(ctx, "sfo_client_test0001"); !ok {
		t.Fatal("client must still exist")
	}

	// 持有管理权限的 actor 可以删除。
	admin := testRequest(routeClientsDeleteID, map[string]any{"clientId": "sfo_client_test0001"})
	admin.Context.Actor = &protocolwire.Actor{UserId: 42, PermissionKeys: []string{permissionManageClients}}
	response, err = server.handleClientDelete(ctx, admin)
	if err != nil {
		t.Fatalf("admin delete failed: %v", err)
	}
	if hasError(response) {
		t.Fatalf("admin delete must pass: %#v", errorReason(t, response))
	}
	if _, ok := server.store.getClient(ctx, "sfo_client_test0001"); ok {
		t.Fatal("client must be deleted")
	}
}

func TestConsentFlowIssuesCode(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	requestToken, err := generateToken(prefixConsentRequest)
	if err != nil {
		t.Fatalf("token generation failed: %v", err)
	}
	if err := server.store.putRequest(ctx, requestRecord{
		TokenHash: hashToken(requestToken), ClientID: "sfo_client_test0001", UserID: 7,
		RedirectURI: "https://app.example.com/callback", Scope: scopeProfile, State: "state-1",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("put request: %v", err)
	}
	summary, err := server.handleConsentSummary(ctx, testRequest(routeConsentSummaryID, map[string]any{}))
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	// 查询参数由 Host 传入，这里直接构造。
	summaryRequest := testRequest(routeConsentSummaryID, map[string]any{})
	summaryRequest.QueryParameters = map[string]string{"request": requestToken}
	summary, err = server.handleConsentSummary(ctx, summaryRequest)
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	values := responseValues(t, summary)
	if values["status"] != "ok" {
		t.Fatalf("summary status = %v (%v)", values["status"], values)
	}
	if values["clientName"] != "Test App" {
		t.Fatalf("client name = %v", values["clientName"])
	}

	decision, err := server.handleConsentDecision(ctx, testRequest(routeConsentDecisionID, map[string]any{
		"request": requestToken, "decision": "allow", "remember": true,
	}))
	if err != nil {
		t.Fatalf("decision failed: %v", err)
	}
	decisionValues := responseValues(t, decision)
	location, _ := decisionValues["location"].(string)
	if !strings.Contains(location, "code=") || !strings.Contains(location, "state=state-1") {
		t.Fatalf("decision location = %v", location)
	}
	if _, ok := server.store.getConsent(ctx, 7, "sfo_client_test0001"); !ok {
		t.Fatal("remembered consent must be stored")
	}
	// 待确认请求是一次性的。
	if _, ok := server.store.getRequest(ctx, hashToken(requestToken)); ok {
		t.Fatal("consent request must be consumed")
	}
	_ = summary
}

// TestClientCreationPermissionMatrix 覆盖 create_clients 权限与总闸（allow_user_clients）的组合，
// 以及「管理自己已有的应用不受总闸影响」「撤销授权不要求应用管理权限」这两条边界。
func TestClientCreationPermissionMatrix(t *testing.T) {
	ctx := context.Background()

	serverWithGate := func(t *testing.T, allowUserApps bool) *oauthServer {
		t.Helper()
		server := testServer(t)
		cfg := defaultSettings()
		cfg.AllowUserApps = allowUserApps
		server.settingsCache = &cfg
		return server
	}
	withActor := func(routeID string, userID int64, permissions []string, values map[string]any) *pluginwire.RouteRequest {
		request := testRequest(routeID, values)
		request.Context.Actor = &protocolwire.Actor{UserId: userID, PermissionKeys: permissions}
		return request
	}
	createBody := map[string]any{
		"name": "Self Service App", "clientType": "public",
		"redirectUris": []any{"https://app.example.com/callback"},
	}
	updateBody := map[string]any{
		"clientId": "sfo_client_test0001", "name": "Renamed",
		"redirectUris": []any{"https://app.example.com/callback"},
	}
	assertForbidden := func(t *testing.T, response *pluginwire.RouteResponse, reason string) {
		t.Helper()
		if !hasError(response) {
			t.Fatalf("expected %s, got a success response", reason)
		}
		if got := errorReason(t, response); got != reason {
			t.Fatalf("reason = %q, want %q", got, reason)
		}
	}

	t.Run("仅登录无权限：列表与创建都拒绝", func(t *testing.T) {
		server := serverWithGate(t, true)
		list, err := server.handleClientsList(ctx, withActor(routeClientsListID, 7, nil, map[string]any{}))
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		assertForbidden(t, list, "oauth2.client_list_forbidden")
		create, err := server.handleClientCreate(ctx, withActor(routeClientsCreateID, 7, nil, createBody))
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		assertForbidden(t, create, "oauth2.client_creation_forbidden")
	})

	t.Run("持权且总闸开：只看得到自己的，可创建", func(t *testing.T) {
		server := serverWithGate(t, true)
		seedClient(t, server, "confidential", "sfos_secret_value") // owner = 7
		list, err := server.handleClientsList(ctx,
			withActor(routeClientsListID, 7, []string{permissionCreateClients}, map[string]any{}))
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if hasError(list) {
			t.Fatalf("list must pass: %#v", errorReason(t, list))
		}
		values := responseValues(t, list)
		if values["canCreate"] != true || values["canManageAll"] != false {
			t.Fatalf("flags = %#v", values)
		}
		clients, _ := values["clients"].([]any)
		if len(clients) != 1 {
			t.Fatalf("holder must only see own clients: %#v", values["clients"])
		}
		create, err := server.handleClientCreate(ctx,
			withActor(routeClientsCreateID, 7, []string{permissionCreateClients}, createBody))
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		if hasError(create) {
			t.Fatalf("create must pass: %#v", errorReason(t, create))
		}
	})

	t.Run("持权但总闸关：不能新建，仍可管理既有应用", func(t *testing.T) {
		server := serverWithGate(t, false)
		seedClient(t, server, "confidential", "sfos_secret_value")
		list, err := server.handleClientsList(ctx,
			withActor(routeClientsListID, 7, []string{permissionCreateClients}, map[string]any{}))
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if hasError(list) {
			t.Fatalf("list must pass with the gate closed: %#v", errorReason(t, list))
		}
		if responseValues(t, list)["canCreate"] != false {
			t.Fatal("canCreate must be false when the site gate is closed")
		}
		create, err := server.handleClientCreate(ctx,
			withActor(routeClientsCreateID, 7, []string{permissionCreateClients}, createBody))
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		assertForbidden(t, create, "oauth2.client_creation_disabled")
		update, err := server.handleClientUpdate(ctx,
			withActor(routeClientsUpdateID, 7, []string{permissionCreateClients}, updateBody))
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
		if hasError(update) {
			t.Fatalf("owner update must survive a closed creation gate: %#v", errorReason(t, update))
		}
	})

	t.Run("管理员：看全部且不受总闸限制", func(t *testing.T) {
		server := serverWithGate(t, false)
		seedClient(t, server, "confidential", "sfos_secret_value") // owner = 7，管理员是 42
		list, err := server.handleClientsList(ctx,
			withActor(routeClientsListID, 42, []string{permissionManageClients}, map[string]any{}))
		if err != nil {
			t.Fatalf("list failed: %v", err)
		}
		if hasError(list) {
			t.Fatalf("admin list must pass: %#v", errorReason(t, list))
		}
		values := responseValues(t, list)
		if values["canManageAll"] != true || values["canCreate"] != true {
			t.Fatalf("admin flags = %#v", values)
		}
		clients, _ := values["clients"].([]any)
		if len(clients) != 1 {
			t.Fatalf("admin must see other owners' clients: %#v", values["clients"])
		}
		create, err := server.handleClientCreate(ctx,
			withActor(routeClientsCreateID, 42, []string{permissionManageClients}, createBody))
		if err != nil {
			t.Fatalf("create failed: %v", err)
		}
		if hasError(create) {
			t.Fatalf("admin create must pass with a closed gate: %#v", errorReason(t, create))
		}
	})

	t.Run("持权用户不能改他人应用", func(t *testing.T) {
		server := serverWithGate(t, true)
		seedClient(t, server, "confidential", "sfos_secret_value") // owner = 7
		update, err := server.handleClientUpdate(ctx,
			withActor(routeClientsUpdateID, 99, []string{permissionCreateClients}, updateBody))
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
		assertForbidden(t, update, "oauth2.client_forbidden")
	})

	t.Run("权限被收回后连自己的应用也改不动", func(t *testing.T) {
		server := serverWithGate(t, true)
		seedClient(t, server, "confidential", "sfos_secret_value") // owner = 7
		update, err := server.handleClientUpdate(ctx, withActor(routeClientsUpdateID, 7, nil, updateBody))
		if err != nil {
			t.Fatalf("update failed: %v", err)
		}
		assertForbidden(t, update, "oauth2.client_forbidden")
	})

	t.Run("撤销自己的授权不要求应用管理权限", func(t *testing.T) {
		server := serverWithGate(t, true)
		response, err := server.handleAuthorizationRevoke(ctx,
			withActor(routeAuthorizationsRevok, 7, nil, map[string]any{"clientId": "sfo_client_test0001"}))
		if err != nil {
			t.Fatalf("revoke failed: %v", err)
		}
		if hasError(response) {
			t.Fatalf("revoking own authorization must not require client permissions: %#v", errorReason(t, response))
		}
	})
}


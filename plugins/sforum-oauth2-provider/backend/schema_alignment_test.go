package main

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// Host 会对每个 HTTP 路由响应做 JSON Schema 校验，所以 handler 的输出必须与
// schemas/*.json 完全一致。这里直接校验真实响应，避免"契约漂移"在启用后才暴露。

func compileSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "schemas", name))
	if err != nil {
		t.Fatalf("resolve schema: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", name, err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("parse schema %s: %v", name, err)
	}
	location := "file://" + filepath.ToSlash(path)
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(location, document); err != nil {
		t.Fatalf("add schema %s: %v", name, err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	return schema
}

func assertMatchesSchema(t *testing.T, name string, response *pluginwire.RouteResponse) {
	t.Helper()
	if response == nil {
		t.Fatalf("%s: response is nil", name)
	}
	if response.GetError() != nil {
		// 错误现在以终端响应（body）返回；只带 Error 字段会被宿主判成
		// 502 extensions.route_unavailable，因此这里明确拒绝这种形态。
		t.Fatalf("%s: routeFailure must return a terminal response, not Error-only", name)
	}
	values := responseValues(t, response)
	schema := compileSchema(t, name)
	if err := schema.Validate(values); err != nil {
		t.Fatalf("%s: response does not match schema: %v\npayload: %#v", name, err, values)
	}
}

func TestCSRFHandshakeMatchesSchema(t *testing.T) {
	server := testServer(t)
	response, err := server.handleCSRF(testRequest(routeCSRFID, map[string]any{}))
	if err != nil {
		t.Fatalf("csrf: %v", err)
	}
	assertMatchesSchema(t, "csrf.response.json", response)
}

func TestAuthorizeResponsesMatchSchema(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()

	request := testRequest(routeAuthorizeID, map[string]any{})
	request.Method = "GET"
	request.Path = "/api/oauth/authorize"
	request.QueryParameters = map[string]string{
		"client_id": "sfo_client_test0001", "redirect_uri": "https://app.example.com/callback",
		"response_type": "code", "scope": scopeProfile, "state": "abc",
	}
	// 已登录用户首次授权（require_consent 默认开启）→ 跳转确认页。
	response, err := server.handleAuthorize(ctx, request)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("expected consent redirect, got %d %#v", response.GetStatusCode(), response)
	}
	assertMatchesSchema(t, "authorize.response.json", response)
	// 确认页地址必须带回原始授权请求：确认页的「切换账号」退出后要回到同一请求。
	consentLocation := responseValues(t, response)["location"]
	consentTarget, _ := consentLocation.(string)
	if !strings.Contains(consentTarget, "/api/oauth/consent?request=") || !strings.Contains(consentTarget, "&return=") {
		t.Fatalf("consent redirect must carry the original authorize request, got %q", consentTarget)
	}
	parsedTarget, err := url.Parse(consentTarget)
	if err != nil {
		t.Fatalf("parse consent redirect: %v", err)
	}
	if got := parsedTarget.Query().Get("return"); got != "/api/oauth/authorize?client_id=sfo_client_test0001&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback&response_type=code&scope=profile&state=abc" {
		t.Fatalf("consent return must round-trip the authorize request, got %q", got)
	}

	// 未登录用户 → 跳转站内登录页。
	anonymous := testRequest(routeAuthorizeID, map[string]any{})
	anonymous.Method = "GET"
	anonymous.Path = "/api/oauth/authorize"
	anonymous.QueryParameters = request.QueryParameters
	anonymous.Context.Actor = nil
	response, err = server.handleAuthorize(ctx, anonymous)
	if err != nil {
		t.Fatalf("authorize anonymous: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("expected login redirect, got %d", response.GetStatusCode())
	}
	assertMatchesSchema(t, "authorize.response.json", response)

	// 非法 redirect_uri 不能回跳，只能返回结构化错误。
	invalid := testRequest(routeAuthorizeID, map[string]any{})
	invalid.Method = "GET"
	invalid.QueryParameters = map[string]string{
		"client_id": "sfo_client_test0001", "redirect_uri": "https://evil.example.com/callback",
		"response_type": "code",
	}
	response, err = server.handleAuthorize(ctx, invalid)
	if err != nil {
		t.Fatalf("authorize invalid: %v", err)
	}
	if !hasError(response) {
		t.Fatal("unregistered redirect_uri must fail closed instead of redirecting")
	}
}

func TestConsentResponsesMatchSchema(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	requestToken, err := generateToken(prefixConsentRequest)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if err := server.store.putRequest(ctx, requestRecord{
		TokenHash: hashToken(requestToken), ClientID: "sfo_client_test0001", UserID: 7,
		RedirectURI: "https://app.example.com/callback", Scope: scopeProfile, State: "s1",
		ExpiresAt: "2099-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("put request: %v", err)
	}

	summary := testRequest(routeConsentSummaryID, map[string]any{})
	summary.Method = "GET"
	summary.QueryParameters = map[string]string{"request": requestToken}
	response, err := server.handleConsentSummary(ctx, summary)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	assertMatchesSchema(t, "consent-summary.response.json", response)
	summaryValues := responseValues(t, response)
	if summaryValues["status"] != "ok" {
		t.Fatalf("expected ok summary, got %#v", summaryValues)
	}
	// 统一 icon 规范：确认页 summary 必须下发与 clients/authorizations 同构的描述符。
	seededIcon, _ := summaryValues["icon"].(map[string]any)
	if seededIcon == nil || seededIcon["kind"] != "generated" || seededIcon["initials"] == "" {
		t.Fatalf("consent summary must carry the unified icon descriptor, got %#v", summaryValues["icon"])
	}

	expired := testRequest(routeConsentSummaryID, map[string]any{})
	expired.Method = "GET"
	expired.QueryParameters = map[string]string{"request": "sfoq_does_not_exist"}
	response, err = server.handleConsentSummary(ctx, expired)
	if err != nil {
		t.Fatalf("summary expired: %v", err)
	}
	assertMatchesSchema(t, "consent-summary.response.json", response)

	decision, err := server.handleConsentDecision(ctx, testRequest(routeConsentDecisionID, map[string]any{
		"request": requestToken, "decision": "allow", "remember": true,
	}))
	if err != nil {
		t.Fatalf("decision: %v", err)
	}
	assertMatchesSchema(t, "consent-decision.response.json", decision)

	denied := testRequest(routeConsentDecisionID, map[string]any{
		"request": "sfoq_does_not_exist", "decision": "deny",
	})
	response, err = server.handleConsentDecision(ctx, denied)
	if err != nil {
		t.Fatalf("decision denied: %v", err)
	}
	assertMatchesSchema(t, "consent-decision.response.json", response)
}

func TestTokenResponsesMatchSchema(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	client, _ := server.store.getClient(ctx, "sfo_client_test0001")
	code, err := server.issueAuthorizationCode(ctx, client, 7, scopeProfile, authorizeQuery{
		ClientID: client.ClientID, RedirectURI: "https://app.example.com/callback",
	})
	if err != nil {
		t.Fatalf("issue code: %v", err)
	}
	success, err := server.handleToken(ctx, testRequest(routeTokenID, map[string]any{
		"grant_type": "authorization_code", "client_id": client.ClientID,
		"client_secret": "sfos_secret_value", "code": code,
		"redirect_uri": "https://app.example.com/callback",
	}))
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	assertMatchesSchema(t, "token.response.json", success)

	failure, err := server.handleToken(ctx, testRequest(routeTokenID, map[string]any{
		"grant_type": "client_credentials", "client_id": client.ClientID,
		"client_secret": "sfos_secret_value",
	}))
	if err != nil {
		t.Fatalf("token unsupported grant: %v", err)
	}
	assertMatchesSchema(t, "token.response.json", failure)
	if failure.GetStatusCode() != 400 {
		t.Fatalf("unsupported grant must be 400, got %d", failure.GetStatusCode())
	}
}

func TestUserinfoResponseMatchesSchema(t *testing.T) {
	server := testServer(t)
	request := testRequest(routeUserinfoID, map[string]any{})
	request.Method = "GET"
	request.QueryParameters = map[string]string{"access_token": "sfoa_unknown"}
	response, err := server.handleUserinfo(context.Background(), request)
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	assertMatchesSchema(t, "userinfo.response.json", response)

	// 成功分支需要 Host 用户投影，这里校验 schema 接受成功负载本身。
	// 账号时间线与主页链接都是可选字段（宿主可能给不出来），形状必须是这两种之一。
	success := map[string]any{
		"sub": "7", "id": 7, "username": "alice", "displayName": "Alice", "status": "active",
		"scope": scopeProfile, "clientId": "sfo_client_test0001",
		"createdAt":     "2026-01-02T03:04:05.123456789Z",
		"updatedAt":     "2026-01-02T03:04:05Z",
		"profileUrl":    "https://forum.example.com/u/alice",
		"email":         "alice@example.com",
		"emailVerified": true,
	}
	if err := compileSchema(t, "userinfo.response.json").Validate(success); err != nil {
		t.Fatalf("userinfo success payload must match schema: %v", err)
	}
	// id 是数字形态（与 GitHub 一致）；字符串形态属于契约外，必须被拒。
	stringID := map[string]any{
		"sub": "7", "id": "7", "username": "alice", "displayName": "Alice", "status": "active",
		"scope": scopeProfile, "clientId": "sfo_client_test0001",
	}
	if err := compileSchema(t, "userinfo.response.json").Validate(stringID); err == nil {
		t.Fatal("userinfo id must stay numeric; a string id must be rejected by the schema")
	}

	// 头像兜底是 data URL；契约只要求 string，这里显式守住，避免以后有人给
	// avatar 加 format 约束把默认安装的用户挡在门外。
	withAvatar := map[string]any{
		"sub": "7", "username": "alice", "displayName": "Alice", "status": "active",
		"scope": "profile avatar", "clientId": "sfo_client_test0001",
		"avatar": generatedAvatarDataURL("Alice"),
	}
	if err := compileSchema(t, "userinfo.response.json").Validate(withAvatar); err != nil {
		t.Fatalf("userinfo avatar payload must match schema: %v", err)
	}
}

func TestRevokeAndIntrospectMatchSchema(t *testing.T) {
	server := testServer(t)
	seedClient(t, server, "confidential", "sfos_secret_value")
	ctx := context.Background()
	client, _ := server.store.getClient(ctx, "sfo_client_test0001")
	pair, err := server.issueTokens(ctx, client.ClientID, 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	revoked, err := server.handleRevoke(ctx, testRequest(routeRevokeID, map[string]any{
		"token": pair.RefreshToken, "client_id": client.ClientID, "client_secret": "sfos_secret_value",
	}))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	assertMatchesSchema(t, "revoke.response.json", revoked)

	introspected, err := server.handleIntrospect(ctx, testRequest(routeIntrospectID, map[string]any{
		"token": pair.AccessToken, "client_id": client.ClientID, "client_secret": "sfos_secret_value",
	}))
	if err != nil {
		t.Fatalf("introspect: %v", err)
	}
	assertMatchesSchema(t, "introspect.response.json", introspected)
}

func TestClientManagementMatchesSchema(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	request := testRequest(routeClientsCreateID, map[string]any{
		"name": "Local App", "description": "from test", "clientType": "confidential",
		"redirectUris": []any{"https://local.example.com/callback"},
	})
	created, err := server.handleClientCreate(ctx, request)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	assertMatchesSchema(t, "client-write.response.json", created)
	createdValues := responseValues(t, created)
	clientID, _ := createdValues["client"].(map[string]any)["clientId"].(string)
	if clientID == "" {
		t.Fatalf("created client id missing: %#v", createdValues)
	}
	if secret, _ := createdValues["clientSecret"].(string); secret == "" {
		t.Fatal("confidential client creation must return the secret once")
	}
	// 未设置 logo：统一规范生成 icon（kind=generated + GitHub 风格 identicon）。
	createdClient, _ := createdValues["client"].(map[string]any)
	createdIcon, _ := createdClient["icon"].(map[string]any)
	if createdIcon == nil || createdIcon["kind"] != "generated" {
		t.Fatalf("client without logo must carry a generated icon, got %#v", createdClient["icon"])
	}
	if pattern, _ := createdIcon["pattern"].(string); len(pattern) != 25 {
		t.Fatalf("generated icon pattern = %v, want 25 cells", createdIcon["pattern"])
	}
	if createdIcon["background"] != iconBackground || createdIcon["color"] == "" {
		t.Fatalf("generated icon colors = %#v, want background %s + identicon color", createdIcon, iconBackground)
	}

	list, err := server.handleClientsList(ctx, testRequest(routeClientsListID, map[string]any{}))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertMatchesSchema(t, "clients.response.json", list)
	listValues := responseValues(t, list)
	listRows, _ := listValues["clients"].([]any)
	if len(listRows) != 1 {
		t.Fatalf("expected 1 client row, got %#v", listValues["clients"])
	}
	if row, _ := listRows[0].(map[string]any); row != nil {
		if row["icon"] == nil || row["logoUrl"] == nil {
			t.Fatalf("clients list rows must expose logoUrl and icon, got %#v", row)
		}
	}

	update, err := server.handleClientUpdate(ctx, testRequest(routeClientsUpdateID, map[string]any{
		"clientId": clientID, "name": "Local App v2",
		"logoUrl":      "https://cdn.example.com/logo.png",
		"redirectUris": []any{"https://local.example.com/callback", "https://local.example.com/alt"},
	}))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	assertMatchesSchema(t, "client-write.response.json", update)
	if secret, _ := responseValues(t, update)["clientSecret"].(string); secret != "" {
		t.Fatal("update must not re-return the client secret")
	}
	// 设置了 logo：描述符切换为 url 来源，并回传 logoUrl。
	updatedClient, _ := responseValues(t, update)["client"].(map[string]any)
	updatedIcon, _ := updatedClient["icon"].(map[string]any)
	if updatedClient["logoUrl"] != "https://cdn.example.com/logo.png" {
		t.Fatalf("updated logoUrl = %#v", updatedClient["logoUrl"])
	}
	if updatedIcon == nil || updatedIcon["kind"] != iconKindURL || updatedIcon["url"] != "https://cdn.example.com/logo.png" {
		t.Fatalf("updated client must carry a custom icon descriptor, got %#v", updatedClient["icon"])
	}

	rotate, err := server.handleClientRotateSecret(ctx, testRequest(routeClientsRotateID, map[string]any{
		"clientId": clientID,
	}))
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	assertMatchesSchema(t, "client-write.response.json", rotate)
	if secret, _ := responseValues(t, rotate)["clientSecret"].(string); secret == "" {
		t.Fatal("rotate must return the new secret once")
	}

	authorizations, err := server.handleAuthorizationsList(ctx, testRequest(routeAuthorizationsList, map[string]any{}))
	if err != nil {
		t.Fatalf("authorizations: %v", err)
	}
	assertMatchesSchema(t, "authorizations.response.json", authorizations)

	deleted, err := server.handleClientDelete(ctx, testRequest(routeClientsDeleteID, map[string]any{
		"clientId": clientID,
	}))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	assertMatchesSchema(t, "mutation.response.json", deleted)

	revoked, err := server.handleAuthorizationRevoke(ctx, testRequest(routeAuthorizationsRevok, map[string]any{
		"clientId": clientID,
	}))
	if err != nil {
		t.Fatalf("revoke authorization: %v", err)
	}
	assertMatchesSchema(t, "mutation.response.json", revoked)
}

func TestInvalidRedirectURIRejectedOnRegistration(t *testing.T) {
	server := testServer(t)
	response, err := server.handleClientCreate(context.Background(), testRequest(routeClientsCreateID, map[string]any{
		"name": "Bad App", "clientType": "confidential",
		"redirectUris": []any{"http://evil.example.com/callback"},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !hasError(response) {
		t.Fatal("non-https redirect URI must be rejected")
	}
	if got := errorReason(t, response); got != "oauth2.invalid_request" {
		t.Fatalf("reason = %s", got)
	}
}

// TestEveryRouteFailureIsTerminalResponse 守住本次修复的根因：
// 宿主把只带 RouteResponse.Error 的响应判为传输层失败，映射成 502
// extensions.route_unavailable。任何错误分支都必须返回真正的终端响应。
func TestEveryRouteFailureIsTerminalResponse(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()

	cases := []struct {
		name    string
		routeID string
		invoke  func() (*pluginwire.RouteResponse, error)
	}{
		{
			name:    "authorize 未知 client",
			routeID: routeAuthorizeID,
			invoke: func() (*pluginwire.RouteResponse, error) {
				request := testRequest(routeAuthorizeID, map[string]any{})
				request.Method = "GET"
				request.QueryParameters = map[string]string{
					"client_id": "sfo_client_missing", "redirect_uri": "https://evil.example.com/cb",
					"response_type": "code",
				}
				return server.handleAuthorize(ctx, request)
			},
		},
		{
			name:    "clients 列表无权限",
			routeID: routeClientsListID,
			invoke: func() (*pluginwire.RouteResponse, error) {
				request := testRequest(routeClientsListID, map[string]any{})
				request.Context.Actor = &protocolwire.Actor{UserId: 7}
				return server.handleClientsList(ctx, request)
			},
		},
		{
			name:    "clients 创建无权限",
			routeID: routeClientsCreateID,
			invoke: func() (*pluginwire.RouteResponse, error) {
				request := testRequest(routeClientsCreateID, map[string]any{
					"name": "App", "clientType": "confidential",
					"redirectUris": []any{"https://app.example.com/cb"},
				})
				request.Context.Actor = &protocolwire.Actor{UserId: 7}
				return server.handleClientCreate(ctx, request)
			},
		},
		{
			name:    "clients 更新未知应用",
			routeID: routeClientsUpdateID,
			invoke: func() (*pluginwire.RouteResponse, error) {
				request := testRequest(routeClientsUpdateID, map[string]any{
					"clientId": "sfo_client_missing", "name": "Renamed",
				})
				request.Context.Actor = &protocolwire.Actor{UserId: 42, PermissionKeys: []string{permissionManageClients}}
				return server.handleClientUpdate(ctx, request)
			},
		},
		{
			name:    "clients 删除未知应用",
			routeID: routeClientsDeleteID,
			invoke: func() (*pluginwire.RouteResponse, error) {
				request := testRequest(routeClientsDeleteID, map[string]any{"clientId": "sfo_client_missing"})
				request.Context.Actor = &protocolwire.Actor{UserId: 42, PermissionKeys: []string{permissionManageClients}}
				return server.handleClientDelete(ctx, request)
			},
		},
		{
			name:    "未知 route id",
			routeID: "sforum.oauth2-provider.route.does-not-exist",
			invoke: func() (*pluginwire.RouteResponse, error) {
				return server.InvokeRoute(ctx, testRequest("sforum.oauth2-provider.route.does-not-exist", map[string]any{}))
			},
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			response, err := item.invoke()
			if err != nil {
				t.Fatalf("invoke: %v", err)
			}
			if response == nil {
				t.Fatal("response is nil")
			}
			if response.GetError() != nil {
				t.Fatalf("route %s returned Error-only response; Host maps it to 502 extensions.route_unavailable",
					item.routeID)
			}
			if response.GetStatusCode() < 100 || response.GetStatusCode() > 599 {
				t.Fatalf("route %s must carry a terminal status, got %d", item.routeID, response.GetStatusCode())
			}
			if response.GetBody() == nil || response.GetBody().GetValue() == nil {
				t.Fatalf("route %s must carry a typed body matching its responseSchema", item.routeID)
			}
			values := response.GetBody().GetValue().AsMap()
			if _, ok := values["error"].(string); !ok {
				t.Fatalf("route %s error body must expose an error code, got %#v", item.routeID, values)
			}
			// Content-Type 必须是单个 application/json，否则宿主 schema 校验直接失败。
			types := map[string]int{}
			for _, header := range response.GetHeaders() {
				if strings.EqualFold(header.GetName(), "Content-Type") {
					for _, value := range header.GetValues() {
						types[value]++
					}
				}
			}
			if len(types) != 1 {
				t.Fatalf("route %s must send exactly one Content-Type, got %#v", item.routeID, types)
			}
		})
	}
}

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
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
		// 结构化错误不会经过 JSON Schema 校验，这里只确认 reason 稳定。
		if response.GetError().GetReason() == "" {
			t.Fatalf("%s: structured error without reason", name)
		}
		return
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
	if response.GetError() == nil {
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
	success := map[string]any{
		"sub": "7", "username": "alice", "displayName": "Alice", "status": "active",
		"scope": scopeProfile, "clientId": "sfo_client_test0001",
	}
	if err := compileSchema(t, "userinfo.response.json").Validate(success); err != nil {
		t.Fatalf("userinfo success payload must match schema: %v", err)
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

	list, err := server.handleClientsList(ctx, testRequest(routeClientsListID, map[string]any{}))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	assertMatchesSchema(t, "clients.response.json", list)

	update, err := server.handleClientUpdate(ctx, testRequest(routeClientsUpdateID, map[string]any{
		"clientId": clientID, "name": "Local App v2",
		"redirectUris": []any{"https://local.example.com/callback", "https://local.example.com/alt"},
	}))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	assertMatchesSchema(t, "client-write.response.json", update)
	if secret, _ := responseValues(t, update)["clientSecret"].(string); secret != "" {
		t.Fatal("update must not re-return the client secret")
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
	if response.GetError() == nil {
		t.Fatal("non-https redirect URI must be rejected")
	}
	if response.GetError().GetReason() != "oauth2.invalid_request" {
		t.Fatalf("reason = %s", response.GetError().GetReason())
	}
}

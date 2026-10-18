package main

import (
	"context"
	"testing"
	"time"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// email 快照链路：授权码签发时用宿主的一次性委派读本人邮箱并落库，userinfo 只读快照。
// 这组用例守住：请求形状与宿主合同一致、只在授予 email 时写、收窄/撤销即删、
// 读取失败不影响授权、以及未授予时字段缺席。

const (
	testEmailClientID = "sfo_client_email01"
	testEmailSecret   = "sfos_email_secret"
)

func seedEmailClient(t *testing.T, server *oauthServer) clientRecord {
	t.Helper()
	record := clientRecord{
		ClientID: testEmailClientID, Name: "Email App", ClientType: "confidential",
		SecretHash:   hashToken(testEmailSecret),
		RedirectURIs: []string{"https://app.example.com/callback"},
		Scopes:       []string{scopeProfile, scopeEmail}, OwnerUserID: 7,
		CreatedAt: "2026-01-01T00:00:00Z",
	}
	if err := server.store.putClient(context.Background(), record); err != nil {
		t.Fatalf("put client: %v", err)
	}
	return record
}

// withSelfEmailDelegation 模拟宿主在路由调用上下文里附上的单次委派。
func withSelfEmailDelegation(request *pluginwire.RouteRequest) *pluginwire.RouteRequest {
	if request.Context == nil {
		request.Context = &protocolwire.RequestContext{}
	}
	request.Context.HostQueryDelegations = []*protocolwire.HostQueryDelegation{{
		QueryId:             pluginv2.SelfEmailQueryID,
		ContractVersion:     pluginv2.SelfEmailContractVersion,
		PlanVersion:         pluginv2.SelfEmailPlanVersion,
		ResultSchemaId:      pluginv2.SelfEmailResultSchemaID,
		ResultSchemaVersion: pluginv2.SelfEmailResultSchemaVersion,
		Scope:               pluginv2.SelfEmailScope,
		Token:               "delegated-self-email-token",
	}}
	return request
}

func selfEmailRows(email string, verified bool) []map[string]any {
	return []map[string]any{{
		"email": email, "email_verified": verified, "updated_at": "2026-10-01T00:00:00Z",
	}}
}

func testServerWithSelfEmail(
	t *testing.T,
	rows []map[string]any,
	detail *protocolwire.ErrorDetail,
) (*oauthServer, *fakeQueryClient) {
	t.Helper()
	server, _ := testServerWithIdentity(t, identityFields(""), false)
	client := &fakeQueryClient{rows: rows, errorDetail: detail}
	server.hostOverride = &pluginv2.Host{Identity: server.hostOverride.Identity, Queries: client}
	server.baseURL = "http://127.0.0.1:3000"
	server.baseURLFetchedAt = time.Now()
	return server, client
}

func pendingConsentRequest(t *testing.T, server *oauthServer, scope string) string {
	t.Helper()
	token := "sfoq_test_request_token"
	if err := server.store.putRequest(context.Background(), requestRecord{
		TokenHash: hashToken(token), ClientID: testEmailClientID, UserID: 7,
		RedirectURI: "https://app.example.com/callback", Scope: scope, State: "st",
		ExpiresAt: time.Now().UTC().Add(10 * time.Minute).Format(timeLayout),
	}); err != nil {
		t.Fatalf("put request: %v", err)
	}
	return token
}

func consentDecisionRequest(t *testing.T, server *oauthServer, requestToken string) *pluginwire.RouteRequest {
	t.Helper()
	request := testRequest(routeConsentDecisionID, map[string]any{
		"request": requestToken, "decision": "allow",
	})
	return withSelfEmailDelegation(request)
}

func TestConsentDecisionCapturesEmailSnapshot(t *testing.T) {
	server, client := testServerWithSelfEmail(t, selfEmailRows("alice@example.com", true), nil)
	seedEmailClient(t, server)
	requestToken := pendingConsentRequest(t, server, "profile email")

	response, err := server.handleConsentDecision(context.Background(), consentDecisionRequest(t, server, requestToken))
	if err != nil {
		t.Fatalf("consent decision: %v", err)
	}
	values := responseValues(t, response)
	if status, _ := values["status"].(string); status != "redirect" {
		t.Fatalf("consent decision status = %#v", values)
	}
	if got := client.count(pluginv2.SelfEmailQueryID); got != 1 {
		t.Fatalf("self-email query calls = %d, want 1", got)
	}
	claim, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID)
	if !ok || claim.Email != "alice@example.com" || !claim.EmailVerified {
		t.Fatalf("email claim = %#v (ok=%v)", claim, ok)
	}

	// userinfo 读快照：授予 email 时下发，值来自落库而不是再次查询宿主。
	pair, err := server.issueTokens(context.Background(), testEmailClientID, 7, "profile email", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	userinfo := userinfoForToken(t, server, pair.AccessToken)
	if got, _ := userinfo["email"].(string); got != "alice@example.com" {
		t.Fatalf("userinfo email = %#v", userinfo["email"])
	}
	if got, _ := userinfo["emailVerified"].(bool); !got {
		t.Fatalf("userinfo emailVerified = %#v", userinfo["emailVerified"])
	}
	if got := client.count(pluginv2.SelfEmailQueryID); got != 1 {
		t.Fatalf("userinfo must not re-query the host for email, self-email calls = %d", got)
	}
}

func TestAuthorizeAutoApproveCapturesEmailSnapshot(t *testing.T) {
	server, client := testServerWithSelfEmail(t, selfEmailRows("alice@example.com", false), nil)
	seedEmailClient(t, server)
	// 已有覆盖 email 的授权 → 不再跳确认页，直接签发授权码。
	if err := server.store.upsertConsent(context.Background(), 7, testEmailClientID, "profile email"); err != nil {
		t.Fatalf("upsert consent: %v", err)
	}
	request := withSelfEmailDelegation(testRequest(routeAuthorizeID, map[string]any{}))
	request.Method = "GET"
	request.Path = "/api/oauth/authorize"
	request.QueryParameters = map[string]string{
		"client_id": testEmailClientID, "redirect_uri": "https://app.example.com/callback",
		"response_type": "code", "scope": "profile email",
	}
	response, err := server.handleAuthorize(context.Background(), request)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("authorize status = %d, want 302", response.GetStatusCode())
	}
	if got := client.count(pluginv2.SelfEmailQueryID); got != 1 {
		t.Fatalf("self-email query calls = %d, want 1", got)
	}
	claim, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID)
	if !ok || claim.Email != "alice@example.com" || claim.EmailVerified {
		t.Fatalf("email claim = %#v (ok=%v)", claim, ok)
	}
}

func TestSelfEmailRequestUsesDelegatedContract(t *testing.T) {
	server, client := testServerWithSelfEmail(t, selfEmailRows("alice@example.com", true), nil)
	seedEmailClient(t, server)
	requestToken := pendingConsentRequest(t, server, "profile email")
	if _, err := server.handleConsentDecision(context.Background(), consentDecisionRequest(t, server, requestToken)); err != nil {
		t.Fatalf("consent decision: %v", err)
	}
	request := client.lastRequest
	if request == nil {
		t.Fatal("self-email query was never issued")
	}
	if request.GetQueryId() != pluginv2.SelfEmailQueryID ||
		request.GetContractVersion() != pluginv2.SelfEmailContractVersion ||
		request.GetPlanVersion() != pluginv2.SelfEmailPlanVersion {
		t.Fatalf("query identity = %q / %q / %q", request.GetQueryId(),
			request.GetContractVersion(), request.GetPlanVersion())
	}
	if request.GetResultSchemaId() != pluginv2.SelfEmailResultSchemaID ||
		request.GetResultSchemaVersion() != pluginv2.SelfEmailResultSchemaVersion {
		t.Fatalf("result schema = %q@%q", request.GetResultSchemaId(), request.GetResultSchemaVersion())
	}
	if request.GetScope() != pluginv2.SelfEmailScope {
		t.Fatalf("scope = %q, want %q", request.GetScope(), pluginv2.SelfEmailScope)
	}
	if request.GetActorDelegation() != "delegated-self-email-token" {
		t.Fatalf("actor delegation = %.20q", request.GetActorDelegation())
	}
	if limit := request.GetPage().GetLimit(); limit != 1 {
		t.Fatalf("page limit = %d, want 1", limit)
	}
	// 插件 → 宿主必须 actorless，且不得把委派回传到 context 里。
	if request.GetContext().GetActor() != nil {
		t.Fatalf("self-email query must not carry an actor: %#v", request.GetContext().GetActor())
	}
	if len(request.GetContext().GetHostQueryDelegations()) != 0 ||
		len(request.GetContext().GetHostCommandDelegations()) != 0 {
		t.Fatalf("delegations must not be echoed in the request context: %#v", request.GetContext())
	}
}

// 宿主只接受 Host-to-plugin 方向的委派：插件把 incoming context 原样回传会被拒
// （protocol v2 actor delegations are Host-to-plugin only）。userinfo 现在会收到
// 同意式读取的委派，所以读身份/时间线时必须把委派从回调上下文里剥掉。
func TestUserinfoStripsInvocationDelegationsFromHostCalls(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, selfEmailRows("alice@example.com", true), nil)
	seedEmailClient(t, server)
	pair, err := server.issueTokens(context.Background(), testEmailClientID, 7, "profile email", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	request := withSelfEmailDelegation(userinfoRequest(pair.AccessToken))
	response, err := server.handleUserinfo(context.Background(), request)
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("userinfo status = %d, body = %#v", response.GetStatusCode(), responseValues(t, response))
	}
	identity := server.hostOverride.Identity.(*fakeIdentityClient)
	if identity.lastContext == nil {
		t.Fatal("identity call must carry a request context")
	}
	if identity.lastContext.GetActor() != nil ||
		len(identity.lastContext.GetHostQueryDelegations()) != 0 ||
		len(identity.lastContext.GetHostCommandDelegations()) != 0 ||
		identity.lastContext.GetHostSessionDelegation() != nil {
		t.Fatalf("outbound identity context must be delegation-free: %#v", identity.lastContext)
	}
}

func TestUserinfoOmitsEmailWithoutScope(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, nil, nil)
	seedEmailClient(t, server)
	if err := server.store.putEmailClaim(context.Background(), emailClaimRecord{
		UserID: 7, ClientID: testEmailClientID, Email: "alice@example.com", EmailVerified: true,
	}); err != nil {
		t.Fatalf("put claim: %v", err)
	}
	pair, err := server.issueTokens(context.Background(), testEmailClientID, 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	for _, field := range []string{"email", "emailVerified"} {
		if _, ok := values[field]; ok {
			t.Fatalf("%s must not be exposed without the email scope: %#v", field, values)
		}
	}
}

func TestUserinfoOmitsEmailWithoutSnapshot(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, nil, nil)
	seedEmailClient(t, server)
	pair, err := server.issueTokens(context.Background(), testEmailClientID, 7, "profile email", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	response, err := server.handleUserinfo(context.Background(), userinfoRequest(pair.AccessToken))
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	assertMatchesSchema(t, "userinfo.response.json", response)
	values := responseValues(t, response)
	for _, field := range []string{"email", "emailVerified"} {
		if _, ok := values[field]; ok {
			t.Fatalf("%s must be omitted when no snapshot exists: %#v", field, values)
		}
	}
	if got, _ := values["username"].(string); got != "alice" {
		t.Fatalf("existing fields must survive a missing snapshot: %#v", values)
	}
}

func TestNarrowedAuthorizationDropsEmailSnapshot(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, selfEmailRows("alice@example.com", true), nil)
	seedEmailClient(t, server)
	if err := server.store.putEmailClaim(context.Background(), emailClaimRecord{
		UserID: 7, ClientID: testEmailClientID, Email: "alice@example.com", EmailVerified: true,
	}); err != nil {
		t.Fatalf("put claim: %v", err)
	}
	// 已授权 profile+email，这次只申请 profile → 不需要确认页，但必须删掉旧快照。
	if err := server.store.upsertConsent(context.Background(), 7, testEmailClientID, "profile email"); err != nil {
		t.Fatalf("upsert consent: %v", err)
	}
	request := withSelfEmailDelegation(testRequest(routeAuthorizeID, map[string]any{}))
	request.Method = "GET"
	request.Path = "/api/oauth/authorize"
	request.QueryParameters = map[string]string{
		"client_id": testEmailClientID, "redirect_uri": "https://app.example.com/callback",
		"response_type": "code", "scope": scopeProfile,
	}
	if _, err := server.handleAuthorize(context.Background(), request); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if _, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID); ok {
		t.Fatal("a narrowed authorization must drop the previous email snapshot")
	}
}

func TestAuthorizationSurvivesEmailQueryFailure(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, nil, &protocolwire.ErrorDetail{
		Reason: "host.query_rate_limited", Message: "rate limit exceeded",
	})
	seedEmailClient(t, server)
	requestToken := pendingConsentRequest(t, server, "profile email")
	response, err := server.handleConsentDecision(context.Background(), consentDecisionRequest(t, server, requestToken))
	if err != nil {
		t.Fatalf("consent decision: %v", err)
	}
	if status, _ := responseValues(t, response)["status"].(string); status != "redirect" {
		t.Fatalf("authorization must still succeed when the email query fails: %#v", responseValues(t, response))
	}
	if _, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID); ok {
		t.Fatal("a failed read must not invent a snapshot")
	}
	if reason := server.lastEmailClaimError(); reason == "" {
		t.Fatal("the failure reason must be recorded for operators")
	}
}

func TestAuthorizationWithoutDelegationDegrades(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, selfEmailRows("alice@example.com", true), nil)
	seedEmailClient(t, server)
	requestToken := pendingConsentRequest(t, server, "profile email")
	// 不带委派：模拟 job/匿名路径或扩展未声明 users.email.read。
	request := testRequest(routeConsentDecisionID, map[string]any{
		"request": requestToken, "decision": "allow",
	})
	response, err := server.handleConsentDecision(context.Background(), request)
	if err != nil {
		t.Fatalf("consent decision: %v", err)
	}
	if status, _ := responseValues(t, response)["status"].(string); status != "redirect" {
		t.Fatalf("authorization must not depend on the delegation: %#v", responseValues(t, response))
	}
	if _, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID); ok {
		t.Fatal("without a delegation no snapshot may be written")
	}
}

func TestRevokeAuthorizationDropsEmailSnapshot(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, nil, nil)
	seedEmailClient(t, server)
	if err := server.store.putEmailClaim(context.Background(), emailClaimRecord{
		UserID: 7, ClientID: testEmailClientID, Email: "alice@example.com", EmailVerified: true,
	}); err != nil {
		t.Fatalf("put claim: %v", err)
	}
	response, err := server.handleAuthorizationRevoke(context.Background(), testRequest(routeAuthorizationsRevok, map[string]any{
		"clientId": testEmailClientID,
	}))
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if !hasError(response) {
		assertMatchesSchema(t, "mutation.response.json", response)
	}
	if _, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID); ok {
		t.Fatal("revoking the authorization must drop the email snapshot")
	}
}

func TestDeleteClientDropsEmailSnapshot(t *testing.T) {
	server, _ := testServerWithSelfEmail(t, nil, nil)
	seedEmailClient(t, server)
	if err := server.store.putEmailClaim(context.Background(), emailClaimRecord{
		UserID: 7, ClientID: testEmailClientID, Email: "alice@example.com", EmailVerified: true,
	}); err != nil {
		t.Fatalf("put claim: %v", err)
	}
	if _, err := server.handleClientDelete(context.Background(), testRequest(routeClientsDeleteID, map[string]any{
		"clientId": testEmailClientID,
	})); err != nil {
		t.Fatalf("delete client: %v", err)
	}
	if _, ok := server.store.getEmailClaim(context.Background(), 7, testEmailClientID); ok {
		t.Fatal("deleting the client must drop its email snapshots")
	}
}

func TestEmailScopeIsOfferedByDefault(t *testing.T) {
	if !scopeSatisfies(joinScopes(defaultSettings().AllowedScopes), scopeEmail) {
		t.Fatalf("email must be an allowed scope by default: %#v", defaultSettings().AllowedScopes)
	}
	if label := scopeLabel(scopeEmail); label == scopeEmail {
		t.Fatal("the email scope must have a human-readable consent label")
	}
}

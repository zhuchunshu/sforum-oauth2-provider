package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	hostwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
	"google.golang.org/grpc"
)

// 图标上传通道的单测：Host Command 用 fake client 顶替，委派由测试直接挂在
// RouteRequest.Context 上（生产由 Host 按 Idempotency-Key 签发）。

const iconTestIdempotencyKey = "test-idempotency-key"

// fakeCommandClient 只实现 Execute；Plan 走不到（插件不做 dry-run）。
type fakeCommandClient struct {
	t       *testing.T
	calls   []*hostwire.CommandRequest
	execute func(*hostwire.CommandRequest) (*hostwire.CommandResult, error)
}

func (f *fakeCommandClient) Plan(
	context.Context,
	*hostwire.CommandRequest,
	...grpc.CallOption,
) (*hostwire.CommandPlan, error) {
	return nil, errors.New("plan is not used by the oauth2 provider")
}

func (f *fakeCommandClient) Execute(
	_ context.Context,
	request *hostwire.CommandRequest,
	_ ...grpc.CallOption,
) (*hostwire.CommandResult, error) {
	f.calls = append(f.calls, request)
	if f.execute == nil {
		return committedResult(f.t, request, nil), nil
	}
	return f.execute(request)
}

func (f *fakeCommandClient) callsFor(commandID string) []*hostwire.CommandRequest {
	matched := make([]*hostwire.CommandRequest, 0, len(f.calls))
	for _, call := range f.calls {
		if call.GetCommandId() == commandID {
			matched = append(matched, call)
		}
	}
	return matched
}

// committedResult 造一个成功结果；output 为空时按命令回一个最小文档。
func committedResult(t *testing.T, request *hostwire.CommandRequest, output map[string]any) *hostwire.CommandResult {
	t.Helper()
	if output == nil {
		switch request.GetCommandId() {
		case attachmentClaimCommandID:
			input := request.GetInput().GetValue().AsMap()
			publicID := stringValue(input["attachmentPublicId"])
			output = map[string]any{
				"planned":        false,
				"publicId":       publicID,
				"url":            attachmentMediaURL(publicID),
				"revision":       "2026-10-18T02:16:17.123456Z",
				"referenceCount": "1",
			}
		default:
			output = map[string]any{"planned": false, "released": "1"}
		}
	}
	schemaRef := "sforum.attachments.extension.release.result@1"
	if request.GetCommandId() == attachmentClaimCommandID {
		schemaRef = "sforum.attachments.extension.claim.result@1"
	}
	document, err := pluginv2.NewTypedDocument(schemaRef, output)
	if err != nil {
		t.Fatalf("build command output: %v", err)
	}
	return &hostwire.CommandResult{
		State:  hostwire.CommandState_COMMAND_STATE_COMMITTED,
		Output: document,
	}
}

func rejectedResult(t *testing.T, reason string) *hostwire.CommandResult {
	t.Helper()
	return &hostwire.CommandResult{
		State: hostwire.CommandState_COMMAND_STATE_REJECTED,
		Error: &protocolwire.ErrorDetail{Reason: reason, Message: "rejected by test"},
	}
}

// iconTestServer 构造注入了 fake Host 命令通道的服务器。
func iconTestServer(t *testing.T, client *fakeCommandClient) *oauthServer {
	t.Helper()
	server := testServer(t)
	client.t = t
	server.hostOverride = &pluginv2.Host{Commands: client}
	return server
}

// iconTestRequest 造一个「带 Idempotency-Key + 两条命令委派」的路由请求。
func iconTestRequest(routeID string, values map[string]any) *pluginwire.RouteRequest {
	request := testRequest(routeID, values)
	request.Context.IdempotencyKey = iconTestIdempotencyKey
	request.Context.HostCommandDelegations = []*protocolwire.HostCommandDelegation{
		{CommandId: attachmentClaimCommandID, CommandVersion: attachmentCommandVersion,
			IdempotencyKey: iconTestIdempotencyKey, Token: "claim-token"},
		{CommandId: attachmentReleaseCommandID, CommandVersion: attachmentCommandVersion,
			IdempotencyKey: iconTestIdempotencyKey, Token: "release-token"},
	}
	return request
}

func clientWriteBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"name":         "Icon App",
		"clientType":   "confidential",
		"redirectUris": []any{"https://app.example.com/callback"},
	}
	for key, value := range extra {
		body[key] = value
	}
	return body
}

func storedClient(t *testing.T, server *oauthServer, clientID string) clientRecord {
	t.Helper()
	record, ok := server.store.getClient(context.Background(), clientID)
	if !ok {
		t.Fatalf("client %s not stored", clientID)
	}
	return record
}

func TestCreateWithUploadedIconClaimsAndClearsLogoURL(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	request := iconTestRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"attachmentPublicId":   "abc123def456",
		"attachmentRevision":   "2026-10-18T02:16:17.123456Z",
		"logoUrl":              "",
		"description":          "",
	}))

	response, err := server.handleClientCreate(context.Background(), request)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%v)", response.GetStatusCode(), responseValues(t, response))
	}
	claims := commands.callsFor(attachmentClaimCommandID)
	if len(claims) != 1 {
		t.Fatalf("claim calls = %d, want 1", len(claims))
	}
	if claims[0].GetExpectedRevision() != "2026-10-18T02:16:17.123456Z" {
		t.Fatalf("expectedRevision = %q", claims[0].GetExpectedRevision())
	}
	if claims[0].GetIdempotencyKey() != iconTestIdempotencyKey {
		t.Fatalf("idempotency key = %q", claims[0].GetIdempotencyKey())
	}
	if claims[0].GetActorDelegation() != "claim-token" {
		t.Fatalf("actor delegation = %q", claims[0].GetActorDelegation())
	}
	input := claims[0].GetInput().GetValue().AsMap()
	if input["attachmentPublicId"] != "abc123def456" {
		t.Fatalf("claim input publicId = %v", input["attachmentPublicId"])
	}
	values := responseValues(t, response)
	client, _ := values["client"].(map[string]any)
	clientID := stringValue(client["clientId"])
	if !strings.HasPrefix(attachmentContextPrefix+clientID, "oauth2-client:sfo_client_") {
		t.Fatalf("clientId = %q", clientID)
	}
	if input["context"] != attachmentContext(clientID) {
		t.Fatalf("claim context = %v, want %q", input["context"], attachmentContext(clientID))
	}
	record := storedClient(t, server, clientID)
	if record.AttachmentPublicID != "abc123def456" {
		t.Fatalf("stored publicId = %q", record.AttachmentPublicID)
	}
	if record.LogoURL != "" {
		t.Fatalf("logoUrl must be cleared when an upload wins, got %q", record.LogoURL)
	}
	icon, _ := client["icon"].(map[string]any)
	if icon["kind"] != iconKindAttachment || icon["url"] != "/media/attachments/abc123def456" {
		t.Fatalf("icon descriptor = %#v", icon)
	}
}

// 上传成功的创建响应必须能过 client-write.response 的 schema：attachmentPublicId
// 与 icon.kind=attachment 都是本版新增的字段，漂移会在这里立刻变红。
func TestCreateWithUploadedIconMatchesSchema(t *testing.T) {
	server := iconTestServer(t, &fakeCommandClient{})
	response, err := server.handleClientCreate(context.Background(), iconTestRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"attachmentPublicId": "abc123def456",
		"attachmentRevision": "2026-10-18T02:16:17.123456Z",
	})))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	assertMatchesSchema(t, "client-write.response.json", response)
	client, _ := responseValues(t, response)["client"].(map[string]any)
	if client["attachmentPublicId"] != "abc123def456" {
		t.Fatalf("attachmentPublicId = %#v", client["attachmentPublicId"])
	}
	icon, _ := client["icon"].(map[string]any)
	if icon["kind"] != iconKindAttachment {
		t.Fatalf("icon = %#v", icon)
	}
}

func TestCreateWithoutIconStaysGenerated(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	response, err := server.handleClientCreate(context.Background(), iconTestRequest(routeClientsCreateID, clientWriteBody(nil)))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatalf("no icon means no host command, got %d calls", len(commands.calls))
	}
	client, _ := responseValues(t, response)["client"].(map[string]any)
	icon, _ := client["icon"].(map[string]any)
	if icon["kind"] != iconKindGenerated {
		t.Fatalf("icon = %#v", icon)
	}
}

func TestCreateWithLogoURLNeverClaims(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	response, err := server.handleClientCreate(context.Background(), iconTestRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"logoUrl": "https://cdn.example.com/logo.png",
	})))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatalf("URL icons must not touch host commands, got %d calls", len(commands.calls))
	}
	client, _ := responseValues(t, response)["client"].(map[string]any)
	icon, _ := client["icon"].(map[string]any)
	if icon["kind"] != iconKindURL || icon["url"] != "https://cdn.example.com/logo.png" {
		t.Fatalf("icon = %#v", icon)
	}
}

func TestCreateRejectsBothIconSources(t *testing.T) {
	server := iconTestServer(t, &fakeCommandClient{})
	response, err := server.handleClientCreate(context.Background(), iconTestRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"attachmentPublicId": "abc123def456",
		"logoUrl":            "https://cdn.example.com/logo.png",
	})))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if response.GetStatusCode() != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.GetStatusCode())
	}
	if reason := errorReason(t, response); reason != "oauth2.invalid_request" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestClaimHostFailuresMapToStableCodesAndDoNotStore(t *testing.T) {
	cases := map[string]struct {
		hostReason string
		wantCode   string
		wantStatus uint32
	}{
		"too large":        {"host.attachment_extension_too_large", "oauth2.icon_too_large", uint32(http.StatusRequestEntityTooLarge)},
		"revision conflict": {"host.attachment_revision_conflict", "oauth2.icon_revision_conflict", uint32(http.StatusConflict)},
		"not found":        {"host.attachment_not_found", "oauth2.icon_attachment_missing", uint32(http.StatusNotFound)},
		"forbidden":        {"host.attachment_extension_forbidden", "oauth2.icon_forbidden", uint32(http.StatusForbidden)},
		"reference limit":  {"host.attachment_extension_reference_limit", "oauth2.icon_reference_limit", uint32(http.StatusConflict)},
		"authority denied": {"host.command_authority_denied", "oauth2.icon_upload_unsupported", uint32(http.StatusConflict)},
		"unknown reason":   {"host.something_new", "oauth2.icon_upload_failed", uint32(http.StatusBadGateway)},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			commands := &fakeCommandClient{execute: func(*hostwire.CommandRequest) (*hostwire.CommandResult, error) {
				return rejectedResult(t, testCase.hostReason), nil
			}}
			server := iconTestServer(t, commands)
			response, err := server.handleClientCreate(context.Background(), iconTestRequest(routeClientsCreateID, clientWriteBody(map[string]any{
				"attachmentPublicId": "abc123def456",
				"attachmentRevision": "2026-10-18T02:16:17.123456Z",
			})))
			if err != nil {
				t.Fatalf("create: %v", err)
			}
			if response.GetStatusCode() != testCase.wantStatus {
				t.Fatalf("status = %d, want %d (%v)", response.GetStatusCode(), testCase.wantStatus, responseValues(t, response))
			}
			if reason := errorReason(t, response); reason != testCase.wantCode {
				t.Fatalf("reason = %q, want %q", reason, testCase.wantCode)
			}
			if len(server.store.clients) != 0 {
				t.Fatalf("a failed claim must not store the client: %#v", server.store.clients)
			}
		})
	}
}

func TestClaimWithoutDelegationIsReadableUnsupported(t *testing.T) {
	server := iconTestServer(t, &fakeCommandClient{})
	request := testRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"attachmentPublicId": "abc123def456",
		"attachmentRevision": "2026-10-18T02:16:17.123456Z",
	}))
	// 旧宿主：带了 Idempotency-Key，但 Host 没有签发任何命令委派。
	request.Context.IdempotencyKey = iconTestIdempotencyKey

	response, err := server.handleClientCreate(context.Background(), request)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if response.GetStatusCode() != http.StatusConflict {
		t.Fatalf("status = %d, want 409", response.GetStatusCode())
	}
	if reason := errorReason(t, response); reason != "oauth2.icon_upload_unsupported" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestClaimWithoutIdempotencyKeyIsExplicit(t *testing.T) {
	server := iconTestServer(t, &fakeCommandClient{})
	request := testRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"attachmentPublicId": "abc123def456",
		"attachmentRevision": "2026-10-18T02:16:17.123456Z",
	}))
	response, err := server.handleClientCreate(context.Background(), request)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if response.GetStatusCode() != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.GetStatusCode())
	}
	if reason := errorReason(t, response); reason != "oauth2.idempotency_key_required" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestSwitchFromUploadToLogoURLReleasesExactlyOnce(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	seed := seedClient(t, server, "confidential", "sfos_secret_value")
	seed.AttachmentPublicID = "abc123def456"
	if err := server.store.putClient(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	response, err := server.handleClientUpdate(context.Background(), iconTestRequest(routeClientsUpdateID, clientWriteBody(map[string]any{
		"clientId": seed.ClientID,
		"logoUrl":  "https://cdn.example.com/new.png",
	})))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("status = %d (%v)", response.GetStatusCode(), responseValues(t, response))
	}
	releases := commands.callsFor(attachmentReleaseCommandID)
	if len(releases) != 1 {
		t.Fatalf("release calls = %d, want 1", len(releases))
	}
	input := releases[0].GetInput().GetValue().AsMap()
	if input["context"] != attachmentContext(seed.ClientID) {
		t.Fatalf("release context = %v", input["context"])
	}
	record := storedClient(t, server, seed.ClientID)
	if record.AttachmentPublicID != "" {
		t.Fatalf("publicId must be cleared, got %q", record.AttachmentPublicID)
	}
	if record.LogoURL != "https://cdn.example.com/new.png" {
		t.Fatalf("logoUrl = %q", record.LogoURL)
	}
	if len(commands.callsFor(attachmentClaimCommandID)) != 0 {
		t.Fatal("switching to a URL must not claim")
	}
}

func TestSwitchFromLogoURLToUploadClearsLogoURL(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	seed := seedClient(t, server, "confidential", "sfos_secret_value")
	seed.LogoURL = "https://cdn.example.com/old.png"
	if err := server.store.putClient(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	response, err := server.handleClientUpdate(context.Background(), iconTestRequest(routeClientsUpdateID, clientWriteBody(map[string]any{
		"clientId":           seed.ClientID,
		"attachmentPublicId": "newpublicid",
		"attachmentRevision": "2026-10-18T02:16:17.123456Z",
	})))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("status = %d (%v)", response.GetStatusCode(), responseValues(t, response))
	}
	record := storedClient(t, server, seed.ClientID)
	if record.AttachmentPublicID != "newpublicid" {
		t.Fatalf("publicId = %q", record.AttachmentPublicID)
	}
	if record.LogoURL != "" {
		t.Fatalf("logoUrl must be cleared, got %q", record.LogoURL)
	}
	if len(commands.callsFor(attachmentReleaseCommandID)) != 0 {
		t.Fatal("URL icons have no server-side side effect, so no release is needed")
	}
}

func TestClearIconReleasesThenClearsBothFields(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	seed := seedClient(t, server, "confidential", "sfos_secret_value")
	seed.AttachmentPublicID = "abc123def456"
	if err := server.store.putClient(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	response, err := server.handleClientUpdate(context.Background(), iconTestRequest(routeClientsUpdateID, clientWriteBody(map[string]any{
		"clientId":           seed.ClientID,
		"attachmentPublicId": "",
		"logoUrl":            "",
	})))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("status = %d (%v)", response.GetStatusCode(), responseValues(t, response))
	}
	if len(commands.callsFor(attachmentReleaseCommandID)) != 1 {
		t.Fatalf("release calls = %d, want 1", len(commands.callsFor(attachmentReleaseCommandID)))
	}
	record := storedClient(t, server, seed.ClientID)
	if record.AttachmentPublicID != "" || record.LogoURL != "" {
		t.Fatalf("icon fields must be empty: %#v", record)
	}
	client, _ := responseValues(t, response)["client"].(map[string]any)
	icon, _ := client["icon"].(map[string]any)
	if icon["kind"] != iconKindGenerated {
		t.Fatalf("icon = %#v", icon)
	}
}

func TestUpdateWithoutIconFieldsKeepsExistingAttachment(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	seed := seedClient(t, server, "confidential", "sfos_secret_value")
	seed.AttachmentPublicID = "abc123def456"
	if err := server.store.putClient(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	response, err := server.handleClientUpdate(context.Background(), iconTestRequest(routeClientsUpdateID, clientWriteBody(map[string]any{
		"clientId": seed.ClientID,
	})))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("status = %d (%v)", response.GetStatusCode(), responseValues(t, response))
	}
	if len(commands.calls) != 0 {
		t.Fatalf("untouched icon must not call any command, got %d", len(commands.calls))
	}
	if record := storedClient(t, server, seed.ClientID); record.AttachmentPublicID != "abc123def456" {
		t.Fatalf("publicId = %q", record.AttachmentPublicID)
	}
}

func TestDeleteReleasesBestEffortAndNeverBlocks(t *testing.T) {
	commands := &fakeCommandClient{execute: func(request *hostwire.CommandRequest) (*hostwire.CommandResult, error) {
		if request.GetCommandId() == attachmentReleaseCommandID {
			return rejectedResult(t, "host.command_actor_permission_denied"), nil
		}
		return committedResult(t, request, nil), nil
	}}
	server := iconTestServer(t, commands)
	seed := seedClient(t, server, "confidential", "sfos_secret_value")
	seed.AttachmentPublicID = "abc123def456"
	if err := server.store.putClient(context.Background(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	response, err := server.handleClientDelete(context.Background(), iconTestRequest(routeClientsDeleteID, map[string]any{
		"clientId": seed.ClientID,
	}))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if response.GetStatusCode() != http.StatusOK {
		t.Fatalf("a failed release must not block deletion, status = %d", response.GetStatusCode())
	}
	if len(commands.callsFor(attachmentReleaseCommandID)) != 1 {
		t.Fatalf("release calls = %d, want 1", len(commands.callsFor(attachmentReleaseCommandID)))
	}
	if _, ok := server.store.getClient(context.Background(), seed.ClientID); ok {
		t.Fatal("client must be deleted")
	}
}

func TestDeleteWithoutAttachmentSkipsRelease(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	seed := seedClient(t, server, "confidential", "sfos_secret_value")

	if _, err := server.handleClientDelete(context.Background(), iconTestRequest(routeClientsDeleteID, map[string]any{
		"clientId": seed.ClientID,
	})); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(commands.calls) != 0 {
		t.Fatalf("no attachment means no release, got %d calls", len(commands.calls))
	}
}

func TestCreateRollsBackClaimWhenStoreWriteFails(t *testing.T) {
	commands := &fakeCommandClient{}
	server := iconTestServer(t, commands)
	server.store = nil // putClient 返回 "store unavailable"

	response, err := server.handleClientCreate(context.Background(), iconTestRequest(routeClientsCreateID, clientWriteBody(map[string]any{
		"attachmentPublicId": "abc123def456",
		"attachmentRevision": "2026-10-18T02:16:17.123456Z",
	})))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if response.GetStatusCode() != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.GetStatusCode())
	}
	if len(commands.callsFor(attachmentReleaseCommandID)) != 1 {
		t.Fatalf("release calls = %d, want 1 (rollback)", len(commands.callsFor(attachmentReleaseCommandID)))
	}
}

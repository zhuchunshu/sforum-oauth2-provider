package main

import (
	"context"
	"errors"
	"testing"
	"time"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	hostwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
	"google.golang.org/grpc"
)

// 注册时间/资料更新时间走宿主稳定查询（sforum.core.safe_user.by_id）。
// 这组用例守住三件事：
//  1. 请求形状必须与宿主合同一致（查询 id、plan 版本、字段白名单、int64 过滤器）；
//  2. 成功时 userinfo 多出 id/createdAt/updatedAt/profileUrl，且过 schema；
//  3. 任何失败（授权未确认、旧宿主、用户非 active、无 Query 通道）都只让字段缺席，
//     既有字段与 200 状态不变。

type fakeQueryClient struct {
	hostwire.HostQueryServiceClient
	rows        []map[string]any
	errorDetail *protocolwire.ErrorDetail
	err         error
	calls       int
	callsByID   map[string]int
	lastRequest *hostwire.QueryRequest
}

// count 返回某个 query id 的调用次数：同一个 fake 同时服务设置查询与业务查询，
// 用例必须按 id 区分，否则会被设置读取的往返干扰。
func (f *fakeQueryClient) count(queryID string) int {
	if f == nil {
		return 0
	}
	return f.callsByID[queryID]
}

func (f *fakeQueryClient) Execute(
	_ context.Context,
	request *hostwire.QueryRequest,
	_ ...grpc.CallOption,
) (*hostwire.QueryResponse, error) {
	f.calls++
	if f.callsByID == nil {
		f.callsByID = map[string]int{}
	}
	f.callsByID[request.GetQueryId()]++
	f.lastRequest = request
	if f.err != nil {
		return nil, f.err
	}
	response := &hostwire.QueryResponse{}
	if f.errorDetail != nil {
		response.Error = f.errorDetail
		return response, nil
	}
	for _, row := range f.rows {
		document, err := newDocument("sforum.core.safe_user@1", row)
		if err != nil {
			return nil, err
		}
		response.Rows = append(response.Rows, document)
	}
	return response, nil
}

// accountRow 是宿主 safe_users 投影的一行：BIGINT 是十进制字符串，
// timestamptz 是 UTC RFC3339Nano（见宿主 v2_query_postgres.go 的编码规则）。
func accountRow(createdAt, updatedAt string) map[string]any {
	return map[string]any{
		"id":         "7",
		"created_at": createdAt,
		"updated_at": updatedAt,
	}
}

func testServerWithAccount(
	t *testing.T,
	rows []map[string]any,
	errorDetail *protocolwire.ErrorDetail,
) (*oauthServer, *fakeQueryClient) {
	t.Helper()
	server, _ := testServerWithIdentity(t, identityFields(""), false)
	client := &fakeQueryClient{rows: rows, errorDetail: errorDetail}
	server.hostOverride = &pluginv2.Host{Identity: server.hostOverride.Identity, Queries: client}
	// 站点公开地址缓存命中：profileUrl 需要绝对地址，单测不能访问真实站点。
	server.baseURL = "http://127.0.0.1:3000"
	server.baseURLFetchedAt = time.Now()
	return server, client
}

func TestUserinfoIncludesAccountTimestampsAndProfileURL(t *testing.T) {
	server, client := testServerWithAccount(t, []map[string]any{
		accountRow("2026-01-02T03:04:05.123456789Z", "2026-02-03T04:05:06Z"),
	}, nil)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	response, err := server.handleUserinfo(context.Background(), userinfoRequest(pair.AccessToken))
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	assertMatchesSchema(t, "userinfo.response.json", response)
	values := responseValues(t, response)

	if got, _ := values["id"].(float64); got != 7 {
		t.Fatalf("id = %#v, want the numeric user id 7", values["id"])
	}
	if got, _ := values["createdAt"].(string); got != "2026-01-02T03:04:05.123456789Z" {
		t.Fatalf("createdAt = %#v", values["createdAt"])
	}
	if got, _ := values["updatedAt"].(string); got != "2026-02-03T04:05:06Z" {
		t.Fatalf("updatedAt = %#v", values["updatedAt"])
	}
	if got, _ := values["profileUrl"].(string); got != "http://127.0.0.1:3000/u/alice" {
		t.Fatalf("profileUrl = %#v", values["profileUrl"])
	}
	if client.calls != 1 {
		t.Fatalf("account query calls = %d, want 1", client.calls)
	}
}

func TestAccountQueryUsesStableContract(t *testing.T) {
	server, client := testServerWithAccount(t, []map[string]any{
		accountRow("2026-01-02T03:04:05Z", "2026-01-02T03:04:05Z"),
	}, nil)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	if _, err := server.handleUserinfo(context.Background(), userinfoRequest(pair.AccessToken)); err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	request := client.lastRequest
	if request == nil {
		t.Fatal("account query was never issued")
	}
	if request.GetQueryId() != querySafeUserByID || request.GetPlanVersion() != queryStableCorePlanVersion {
		t.Fatalf("query = %q@%q", request.GetQueryId(), request.GetPlanVersion())
	}
	if request.GetResultSchemaId() != querySafeUserSchemaID ||
		request.GetResultSchemaVersion() != querySafeUserSchemaVersion {
		t.Fatalf("result schema = %q@%q", request.GetResultSchemaId(), request.GetResultSchemaVersion())
	}
	if got := request.GetFields(); len(got) != 3 ||
		got[0] != "id" || got[1] != "created_at" || got[2] != "updated_at" {
		t.Fatalf("fields = %#v", got)
	}
	filters := request.GetFilters()
	if len(filters) != 1 || filters[0].GetField() != "id" || filters[0].GetOperator() != "eq" {
		t.Fatalf("filters = %#v", filters)
	}
	value := filters[0].GetValue()
	if value.GetSchemaId() != "sforum.core.query.int64" || value.GetSchemaVersion() != "1" {
		t.Fatalf("filter schema = %q@%q", value.GetSchemaId(), value.GetSchemaVersion())
	}
	if raw, _ := value.GetValue().AsMap()["value"].(string); raw != "7" {
		t.Fatalf("filter value = %#v, want the canonical decimal string", value.GetValue().AsMap())
	}
	if limit := request.GetPage().GetLimit(); limit != 1 {
		t.Fatalf("page limit = %d, want 1 for a single-row query", limit)
	}
	// 插件 → 宿主必须 actorless：带 Actor 会被宿主判 protocol v2 actor context is not host-attested。
	if request.GetContext().GetActor() != nil {
		t.Fatalf("account query must not carry an actor: %#v", request.GetContext().GetActor())
	}
	if request.GetContext().GetRequestId() != "test-request" {
		t.Fatalf("request id must be preserved, got %q", request.GetContext().GetRequestId())
	}
}

func TestUserinfoOmitsTimestampsWhenQueryIsDenied(t *testing.T) {
	server, _ := testServerWithAccount(t, nil, &protocolwire.ErrorDetail{
		Reason:  "host.query_core_views_denied",
		Message: "The exact extension artifact has no stable core-view authority.",
	})
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	response, err := server.handleUserinfo(context.Background(), userinfoRequest(pair.AccessToken))
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	assertMatchesSchema(t, "userinfo.response.json", response)
	values := responseValues(t, response)
	for _, field := range []string{"createdAt", "updatedAt"} {
		if _, ok := values[field]; ok {
			t.Fatalf("%s must be omitted when the query is denied: %#v", field, values)
		}
	}
	if got, _ := values["username"].(string); got != "alice" {
		t.Fatalf("existing fields must survive a denied query, got %#v", values)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("status = %d, want 200", response.GetStatusCode())
	}
	if reason := server.lastLifecycleError(); reason == "" || reason == "host.query_core_views_denied: " {
		t.Fatalf("denial reason must be recorded for operators, got %q", reason)
	}
}

func TestUserinfoOmitsTimestampsForInactiveUser(t *testing.T) {
	// safe_users 只含 status='active'：被禁言/停用的用户查不到行，这不是错误。
	server, _ := testServerWithAccount(t, nil, nil)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	for _, field := range []string{"createdAt", "updatedAt"} {
		if _, ok := values[field]; ok {
			t.Fatalf("%s must be omitted when no active row is returned: %#v", field, values)
		}
	}
	if got, _ := values["profileUrl"].(string); got != "http://127.0.0.1:3000/u/alice" {
		t.Fatalf("profileUrl is independent of the query, got %#v", values["profileUrl"])
	}
}

func TestUserinfoOmitsTimestampsWithoutQueryChannel(t *testing.T) {
	// 旧宿主或只注入 Identity 的调用方没有 Query 通道：静默缺席，不记为失败。
	server, _ := testServerWithIdentity(t, identityFields(""), false)
	server.baseURL = "http://127.0.0.1:3000"
	server.baseURLFetchedAt = time.Now()
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	for _, field := range []string{"createdAt", "updatedAt"} {
		if _, ok := values[field]; ok {
			t.Fatalf("%s must be omitted without a query channel: %#v", field, values)
		}
	}
	if reason := server.lastLifecycleError(); reason != "" {
		t.Fatalf("a missing query channel is not an error, got %q", reason)
	}
}

func TestUserinfoOmitsProfileURLWithoutSiteURL(t *testing.T) {
	server, _ := testServerWithAccount(t, []map[string]any{
		accountRow("2026-01-02T03:04:05Z", "2026-01-02T03:04:05Z"),
	}, nil)
	// 站点地址解析失败（缓存空值）：不猜域名，字段整体省略。
	server.baseURL = ""
	server.baseURLFetchedAt = time.Now()
	t.Setenv("APP_URL", "")
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	if _, ok := values["profileUrl"]; ok {
		t.Fatalf("profileUrl must be omitted without a resolved site URL: %#v", values)
	}
	if got, _ := values["createdAt"].(string); got == "" {
		t.Fatalf("createdAt must not depend on the site URL: %#v", values)
	}
}

func TestUserinfoTimestampsParseAsRFC3339Nano(t *testing.T) {
	server, _ := testServerWithAccount(t, []map[string]any{
		accountRow("2026-01-02T03:04:05.987654321Z", "2026-02-03T04:05:06+08:00"),
	}, nil)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	createdAt, _ := values["createdAt"].(string)
	parsed, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		t.Fatalf("createdAt %q is not RFC3339Nano: %v", createdAt, err)
	}
	if !parsed.Equal(time.Date(2026, 1, 2, 3, 4, 5, 987654321, time.UTC)) {
		t.Fatalf("createdAt = %v", parsed)
	}
	// 非 UTC 的入参统一归一成 UTC，避免第三方看到混合时区。
	updatedAt, _ := values["updatedAt"].(string)
	if updatedAt != "2026-02-02T20:05:06Z" {
		t.Fatalf("updatedAt = %q, want the UTC-normalized value", updatedAt)
	}
}

func TestNormalizeTimestampRejectsNonTimestamps(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, ""},
		{"number", float64(1767225600), ""},
		{"garbage", "not-a-timestamp", ""},
		{"blank", "   ", ""},
		{"date only", "2026-01-02", ""},
		{"rfc3339", "2026-01-02T03:04:05Z", "2026-01-02T03:04:05Z"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := normalizeTimestamp(testCase.value); got != testCase.want {
				t.Fatalf("normalizeTimestamp(%#v) = %q, want %q", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestAccountTimestampsTolerateTransportError(t *testing.T) {
	server, client := testServerWithAccount(t, nil, nil)
	client.err = errors.New("connection reset")
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	if _, ok := values["createdAt"]; ok {
		t.Fatalf("createdAt must be omitted on transport failure: %#v", values)
	}
	if reason := server.lastLifecycleError(); reason != "connection reset" {
		t.Fatalf("transport failure must be recorded, got %q", reason)
	}
}

func userinfoRequest(accessToken string) *pluginwire.RouteRequest {
	request := testRequest(routeUserinfoID, map[string]any{})
	request.Method = "GET"
	request.QueryParameters = map[string]string{"access_token": accessToken}
	return request
}

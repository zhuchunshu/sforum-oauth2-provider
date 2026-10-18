package main

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

// TestResolveAuthorizeScopes 覆盖授权端点的三层 scope 约束：
// 缺省回落应用登记范围、站点清单是硬上限、应用登记范围是自我约束。
func TestResolveAuthorizeScopes(t *testing.T) {
	allowed := []string{scopeProfile, scopeAvatar, scopeTopicCount}

	cases := []struct {
		name       string
		requested  string
		registered []string
		want       string
		wantErr    error
	}{
		{
			name:       "缺省回落到应用登记范围",
			registered: []string{scopeProfile, scopeAvatar},
			want:       "profile avatar",
		},
		{
			name: "缺省且应用未登记时只剩 profile",
			want: scopeProfile,
		},
		{
			name:       "显式请求被保留",
			requested:  scopeAvatar,
			registered: []string{scopeProfile, scopeAvatar},
			want:       "profile avatar",
		},
		{
			name:       "重复 scope 去重",
			requested:  "avatar avatar",
			registered: []string{scopeProfile, scopeAvatar},
			want:       "profile avatar",
		},
		{
			name:       "超出应用登记范围被拒",
			requested:  scopeAvatar,
			registered: []string{scopeProfile},
			wantErr:    errScopeNotRegistered,
		},
		{
			name:       "超出站点清单被拒",
			requested:  scopeUserGroups,
			registered: []string{scopeProfile, scopeUserGroups},
			wantErr:    errUnknownScope,
		},
		{
			// 存量应用的 scopes 列可能没有 profile；身份基底不能被这层约束打回。
			name:       "应用登记范围缺 profile 时仍放行 profile",
			registered: []string{scopeAvatar},
			want:       "profile avatar",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveAuthorizeScopes(tc.requested, tc.registered, allowed)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if join := joinScopes(got); join != tc.want {
				t.Fatalf("scopes = %q, want %q", join, tc.want)
			}
		})
	}
}

// TestAuthorizeDefaultsToRegisteredScopes 是本次修复的端到端回归：
// 第三方不带 scope 时，确认页必须列出应用在后台登记的全部权限。
func TestAuthorizeDefaultsToRegisteredScopes(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	record := seedClient(t, server, "confidential", "sfos_secret_value")
	record.Scopes = []string{scopeProfile, scopeAvatar}
	if err := server.store.putClient(ctx, record); err != nil {
		t.Fatalf("put client: %v", err)
	}

	request := testRequest(routeAuthorizeID, map[string]any{})
	request.Method = "GET"
	request.Path = "/api/oauth/authorize"
	request.QueryParameters = map[string]string{
		"client_id": "sfo_client_test0001", "redirect_uri": "https://app.example.com/callback",
		"response_type": "code", "state": "abc",
	}
	response, err := server.handleAuthorize(ctx, request)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if response.GetStatusCode() != 302 {
		t.Fatalf("expected consent redirect, got %d", response.GetStatusCode())
	}
	location, _ := responseValues(t, response)["location"].(string)
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	requestToken := parsed.Query().Get("request")
	if requestToken == "" {
		t.Fatalf("consent request token missing from %q", location)
	}

	summaryRequest := testRequest(routeConsentSummaryID, map[string]any{})
	summaryRequest.QueryParameters = map[string]string{"request": requestToken}
	summary, err := server.handleConsentSummary(ctx, summaryRequest)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	values := responseValues(t, summary)
	scopes, _ := values["scopes"].([]any)
	if len(scopes) != 2 {
		t.Fatalf("consent page scopes = %#v, want profile + avatar", scopes)
	}
	labels := make([]string, 0, len(scopes))
	for _, item := range scopes {
		entry, _ := item.(map[string]any)
		label, _ := entry["label"].(string)
		labels = append(labels, label)
	}
	if labels[0] != "读取你的用户名、昵称、注册时间与账号状态" || labels[1] != "读取你的头像" {
		t.Fatalf("consent labels = %#v", labels)
	}
}

// TestAuthorizeRejectsScopeNotRegisteredOnClient 保证后台的权限勾选是有效约束，
// 而不是一张没人读取的配置。
func TestAuthorizeRejectsScopeNotRegisteredOnClient(t *testing.T) {
	server := testServer(t)
	ctx := context.Background()
	seedClient(t, server, "confidential", "sfos_secret_value")

	request := testRequest(routeAuthorizeID, map[string]any{})
	request.Method = "GET"
	request.Path = "/api/oauth/authorize"
	request.QueryParameters = map[string]string{
		"client_id": "sfo_client_test0001", "redirect_uri": "https://app.example.com/callback",
		"response_type": "code", "scope": scopeAvatar, "state": "abc",
	}
	response, err := server.handleAuthorize(ctx, request)
	if err != nil {
		t.Fatalf("authorize: %v", err)
	}
	location, _ := responseValues(t, response)["location"].(string)
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if got := parsed.Query().Get("error"); got != "invalid_scope" {
		t.Fatalf("error = %q (location %q)", got, location)
	}
	if len(server.store.requests) != 0 {
		t.Fatalf("rejected authorize must not leave a pending consent request: %#v", server.store.requests)
	}
}

// TestScopeRegistryLabels 守住确认页文案：每个登记过的 scope 都有人话说明，
// 新增 scope 时忘记写 LabelZh 会在这里失败。
func TestScopeRegistryLabels(t *testing.T) {
	for _, def := range scopeRegistry {
		if strings.TrimSpace(def.LabelZh) == "" {
			t.Fatalf("scope %q has no consent label", def.Name)
		}
		if def.LabelZh == def.Name {
			t.Fatalf("scope %q label must not repeat the key", def.Name)
		}
		if strings.HasSuffix(def.LabelZh, "。") {
			t.Fatalf("scope %q label must not end with a full stop (labels are joined by 、)", def.Name)
		}
		if got := scopeLabel(def.Name); got != def.LabelZh {
			t.Fatalf("scopeLabel(%q) = %q, want %q", def.Name, got, def.LabelZh)
		}
	}
	if got := scopeLabel("legacy.unknown"); got != "legacy.unknown" {
		t.Fatalf("unknown scope must fall back to its key, got %q", got)
	}
}

// TestUserinfoGroupsEncodeAsJSONArray 守住 structpb 的编码约束：
// []string 放不进文档，会让整条 userinfo 路由 502。
func TestUserinfoGroupsEncodeAsJSONArray(t *testing.T) {
	payload := map[string]any{
		"sub": "1", "username": "alice", "displayName": "Alice", "status": "active",
		"scope": "profile users.groups", "clientId": "sfo_client_test0001",
		"groups": groupNamesToAny([]string{"member", "vip"}),
	}
	document, err := newDocument(schemaUserinfoResponse, payload)
	if err != nil {
		t.Fatalf("groups must encode into the response document: %v", err)
	}
	values := document.GetValue().AsMap()
	groups, ok := values["groups"].([]any)
	if !ok || len(groups) != 2 {
		t.Fatalf("groups = %#v", values["groups"])
	}
	if groups[0] != "member" || groups[1] != "vip" {
		t.Fatalf("groups = %#v", groups)
	}

	// 空列表编码成 []，不是 null：schema 要求 array。
	empty, err := newDocument(schemaUserinfoResponse, map[string]any{
		"sub": "1", "username": "alice", "displayName": "Alice", "status": "active",
		"scope": "profile users.groups", "clientId": "sfo_client_test0001",
		"groups": groupNamesToAny(nil),
	})
	if err != nil {
		t.Fatalf("empty groups must encode: %v", err)
	}
	if raw, ok := empty.GetValue().AsMap()["groups"].([]any); !ok || len(raw) != 0 {
		t.Fatalf("empty groups = %#v", empty.GetValue().AsMap()["groups"])
	}

	// 反向确认：裸 []string 确实编码失败（否则这条回归守卫没有意义）。
	if _, err := newDocument(schemaUserinfoResponse, map[string]any{"groups": []string{"member"}}); err == nil {
		t.Fatal("raw []string must fail structpb encoding; the helper would be unnecessary")
	}
}

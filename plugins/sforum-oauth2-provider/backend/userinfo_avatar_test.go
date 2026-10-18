package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	hostwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
	"google.golang.org/grpc"
)

// userinfo 头像分支的单测：Host Identity 用 fake client 顶替。
// 回归点——授予 avatar 但宿主没有 URL（initials 账号）时，字段必须仍然存在。

type fakeIdentityClient struct {
	hostwire.IdentityServiceClient
	fields       map[string]any
	failFirst    bool
	calls        int
	lastDeclared []string
	lastContext  *protocolwire.RequestContext
}

func (f *fakeIdentityClient) GetUser(
	_ context.Context,
	request *hostwire.IdentityUserRequest,
	_ ...grpc.CallOption,
) (*hostwire.IdentityUserResponse, error) {
	f.calls++
	f.lastDeclared = append([]string(nil), request.GetDeclaredFields()...)
	f.lastContext = request.GetContext()
	if f.failFirst && f.calls == 1 {
		return &hostwire.IdentityUserResponse{Error: &protocolwire.ErrorDetail{
			Reason: "host.identity_unavailable", Message: "first projection failed",
		}}, nil
	}
	document, err := newDocument("sforum.identity.user.safe@1", f.fields)
	if err != nil {
		return nil, err
	}
	return &hostwire.IdentityUserResponse{User: document}, nil
}

func identityFields(avatar string) map[string]any {
	return map[string]any{
		"id":          float64(7),
		"username":    "alice",
		"displayName": "Alice",
		"status":      "active",
		"avatar":      avatar,
		"groups":      []any{},
	}
}

func testServerWithIdentity(t *testing.T, fields map[string]any, failFirst bool) (*oauthServer, *fakeIdentityClient) {
	t.Helper()
	server := testServer(t)
	client := &fakeIdentityClient{fields: fields, failFirst: failFirst}
	server.hostOverride = &pluginv2.Host{Identity: client}
	return server, client
}

func userinfoForToken(t *testing.T, server *oauthServer, accessToken string) map[string]any {
	t.Helper()
	request := testRequest(routeUserinfoID, map[string]any{})
	request.Method = "GET"
	request.QueryParameters = map[string]string{"access_token": accessToken}
	response, err := server.handleUserinfo(context.Background(), request)
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if response.GetStatusCode() != 200 {
		t.Fatalf("userinfo status = %d, want 200", response.GetStatusCode())
	}
	return responseValues(t, response)
}

func TestUserinfoIncludesGeneratedAvatarWhenHostHasNoURL(t *testing.T) {
	server, client := testServerWithIdentity(t, identityFields(""), false)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, "profile avatar", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	avatar, _ := values["avatar"].(string)
	if !strings.HasPrefix(avatar, avatarDataURLPrefix) {
		t.Fatalf("avatar = %.80q, want a generated data URL", avatar)
	}
	if client.calls != 1 {
		t.Fatalf("identity calls = %d, want 1", client.calls)
	}
	for _, field := range []string{"id", "username", "displayName", "status", "avatar", "groups"} {
		if !containsString(client.lastDeclared, field) {
			t.Fatalf("declared fields missing %q: %#v", field, client.lastDeclared)
		}
	}
	decoded := string(mustDecodeDataURL(t, avatar))
	if !strings.Contains(decoded, `fill="#f0f0f0"`) || strings.Contains(decoded, "<text") {
		t.Fatalf("generated avatar must be a text-free GitHub identicon: %s", decoded)
	}
	if !strings.Contains(decoded, identiconColor("Alice")) {
		t.Fatalf("generated avatar must use the display-name seed: %s", decoded)
	}
}

func TestUserinfoUsesHostAvatarURL(t *testing.T) {
	server, _ := testServerWithIdentity(t, identityFields("/media/avatars/abc"), false)
	// 站点公开地址缓存命中：避免单测访问真实站点。
	server.baseURL = "http://127.0.0.1:3000"
	server.baseURLFetchedAt = time.Now()
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, "profile avatar", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	if got, _ := values["avatar"].(string); got != "http://127.0.0.1:3000/media/avatars/abc" {
		t.Fatalf("avatar = %q, want the absolute host URL", got)
	}
}

func TestUserinfoAbsolutizesAvatarWithRuntimeSiteURL(t *testing.T) {
	// 端到端覆盖「宿主相对路径 → 公开选项 site.url → 绝对链接」这条链。
	optionServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != siteURLOptionPath {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"code":200,"message":"OK","data":{"name":"site.url","value":"https://forum.example.com"}}`)
	}))
	t.Setenv("APP_URL", optionServer.URL)

	server, _ := testServerWithIdentity(t, identityFields("/media/avatars/abc"), false)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, "profile avatar", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	if got, _ := values["avatar"].(string); got != "https://forum.example.com/media/avatars/abc" {
		t.Fatalf("avatar = %q, want the runtime site.url joined with the media path", got)
	}
	optionServer.Close()
}

func TestUserinfoOmitsAvatarWithoutScope(t *testing.T) {
	server, _ := testServerWithIdentity(t, identityFields("/media/avatars/abc"), false)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, scopeProfile, "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	if _, ok := values["avatar"]; ok {
		t.Fatalf("avatar must not be exposed without the scope: %#v", values)
	}
}

func TestUserinfoAvatarSurvivesProjectionFallback(t *testing.T) {
	server, client := testServerWithIdentity(t, identityFields(""), true)
	pair, err := server.issueTokens(context.Background(), "sfo_client_test0001", 7, "profile avatar", "", true)
	if err != nil {
		t.Fatalf("issue tokens: %v", err)
	}
	values := userinfoForToken(t, server, pair.AccessToken)
	if got, _ := values["avatar"].(string); !strings.HasPrefix(got, avatarDataURLPrefix) {
		t.Fatalf("avatar = %.80q, want a generated data URL after the fallback call", got)
	}
	if client.calls != 2 {
		t.Fatalf("identity calls = %d, want 2 (declared projection + fallback)", client.calls)
	}
}

package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// oauthServer 承接 Host 路由分发：所有 OAuth2 端点都在 InvokeRoute 内分流。
// 授权确认页走 stream 模式，因此同时实现 StreamRoute。
type oauthServer struct {
	*pluginv2.Server

	store *oauthStore

	settingsMu        sync.RWMutex
	settingsCache     *settings
	settingsFetchedAt time.Time

	profileErrorMu sync.RWMutex
	profileError   string

	// 账号时间线查询（注册时间/资料更新时间）的最近失败原因，只用于日志定位。
	// 查询失败时 userinfo 只缺字段，不报错（见 account.go）。
	lifecycleErrorMu sync.RWMutex
	lifecycleError   string

	// 授权时邮箱快照（scope=email）的最近失败原因，只用于日志定位（见 email_claim.go）。
	emailClaimErrorMu sync.RWMutex
	emailClaimError   string

	// 站点公开地址缓存：userinfo 的 avatar 必须是可访问的绝对地址，而宿主只给
	// 相对媒体路径（/media/avatars/<publicId>）。解析来源见 baseurl.go。
	baseURLMu        sync.Mutex
	baseURL          string
	baseURLFetchedAt time.Time

	// hostOverride 只给单测用：Host Command / Query / Identity 通道需要一个已握手
	// 的 Host，而单测不启动 go-plugin 运行时，所以注入一个
	// &pluginv2.Host{Commands: fake}（或 Identity: fake）。生产恒为 nil。
	hostOverride *pluginv2.Host
}

// hostClient 返回握手后的 Host。生产走 Server.Host()；单测注入 hostOverride。
// 所有插件 → 宿主的调用（设置、用户投影、Host Command）都从这里取通道。
func (s *oauthServer) hostClient() (*pluginv2.Host, error) {
	if s.hostOverride != nil {
		return s.hostOverride, nil
	}
	return s.Server.Host()
}

func newOAuthServer() (*oauthServer, error) {
	server := &oauthServer{
		Server: pluginv2.NewServer(),
		store:  newOAuthStore(),
	}
	server.WithRuntimeStreams(pluginv2.RuntimeStreams{
		Lifecycle: server.runLifecycle,
		Route: func(stream *pluginv2.RouteStream) error {
			return server.handleRouteStream(stream)
		},
	})
	return server, nil
}

// InvokeRoute 是 HTTP 路由入口；非 HTTP 模式的路由（授权确认页）只做 preflight。
func (s *oauthServer) InvokeRoute(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	switch request.GetRouteId() {
	case routeConsentPageID:
		// stream 模式：Host 先要一个无 body 的 preflight，随后打开 RouteStream。
		return &pluginwire.RouteResponse{
			Context:       responseContext(request.GetContext()),
			StatusCode:    uint32(http.StatusOK),
			Headers:       routeHeaders(map[string]string{"Content-Type": "text/html; charset=utf-8"}),
			StreamFollows: true,
		}, nil
	case routeDocsID:
		return s.handleDocs(request)
	case routeAuthorizeID:
		return s.handleAuthorize(ctx, request)
	case routeCSRFID:
		return s.handleCSRF(request)
	case routeConsentSummaryID:
		return s.handleConsentSummary(ctx, request)
	case routeConsentDecisionID:
		return s.handleConsentDecision(ctx, request)
	case routeTokenID:
		return s.handleToken(ctx, request)
	case routeUserinfoID:
		return s.handleUserinfo(ctx, request)
	case routeRevokeID:
		return s.handleRevoke(ctx, request)
	case routeIntrospectID:
		return s.handleIntrospect(ctx, request)
	case routeClientsListID:
		return s.handleClientsList(ctx, request)
	case routeClientsCreateID:
		return s.handleClientCreate(ctx, request)
	case routeClientsUpdateID:
		return s.handleClientUpdate(ctx, request)
	case routeClientsDeleteID:
		return s.handleClientDelete(ctx, request)
	case routeClientsRotateID:
		return s.handleClientRotateSecret(ctx, request)
	case routeAuthorizationsList:
		return s.handleAuthorizationsList(ctx, request)
	case routeAuthorizationsRevok:
		return s.handleAuthorizationRevoke(ctx, request)
	default:
		// 未知路由：宿主不会为它准备 responseSchema，回落到 authorize 契约披露。
		return routeFailure(request.GetContext(), http.StatusNotFound, "oauth2.route_unknown",
			"unknown oauth2-provider route", schemaAuthorizeResponse)
	}
}

// handleCSRF 是给非浏览器客户端的握手端点：安全方法会由 Host CSRF 中间件
// 种下 csrf_ cookie，客户端随后用 X-Csrf-Token 回传即可调用 unsafe 端点。
func (s *oauthServer) handleCSRF(request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	return routeDocument(request.GetContext(), http.StatusOK, schemaCSRFResponse, map[string]any{
		"ok":                    true,
		"csrfRequired":          true,
		"tokenEndpoint":         "/api/oauth/token",
		"authorizationEndpoint": "/api/oauth/authorize",
		"userinfoEndpoint":      "/api/oauth/userinfo",
	})
}

// ---------- 响应辅助 ----------

func newDocument(schemaRef string, values map[string]any) (*protocolwire.TypedDocument, error) {
	return pluginv2.NewTypedDocument(schemaRef, values)
}

func routeDocument(
	ctx *protocolwire.RequestContext,
	status int,
	schemaRef string,
	values map[string]any,
) (*pluginwire.RouteResponse, error) {
	document, err := newDocument(schemaRef, values)
	if err != nil {
		return nil, err
	}
	return &pluginwire.RouteResponse{
		Context: responseContext(ctx), StatusCode: uint32(status),
		Headers: routeHeaders(jsonResponseHeaders), Body: document,
	}, nil
}

// jsonResponseHeaders 是 JSON 文档响应的固定头：Host 的响应契约校验要求
// 恰好一个 JSON Content-Type。
var jsonResponseHeaders = map[string]string{"Content-Type": "application/json"}

// routeHeaders 把插件响应头转换为 wire 形式（Host 仍会做允许列表过滤）。
func routeHeaders(values map[string]string) []*protocolwire.Header {
	result := make([]*protocolwire.Header, 0, len(values))
	for name, value := range values {
		result = append(result, &protocolwire.Header{Name: name, Values: []string{value}})
	}
	return result
}

// routeRedirect 返回 302 + Location。Host 插件响应头允许列表放行 Location，
// body 仍然是一份符合路由 responseSchema 的 JSON 文档。
func routeRedirect(ctx *protocolwire.RequestContext, location string) (*pluginwire.RouteResponse, error) {
	document, err := newDocument(schemaAuthorizeResponse, map[string]any{
		"status": "redirect", "location": location,
	})
	if err != nil {
		return nil, err
	}
	return &pluginwire.RouteResponse{
		Context:    responseContext(ctx),
		StatusCode: uint32(http.StatusFound),
		Headers:    routeHeaders(map[string]string{"Location": location, "Content-Type": "application/json"}),
		Body:       document,
	}, nil
}

// routeFailure 返回结构化错误。HTTP 状态与 reason 都由调用方决定，
// 便于第三方应用按 OAuth2 错误语义处理。
//
// 关键约束：这里必须返回**真正的终端响应**（StatusCode + 符合该路由
// responseSchema 的 Body），不能只填 RouteResponse.Error。宿主把只带 Error
// 的响应视为传输层失败，会映射成 502 extensions.route_unavailable
// （protocol_v2_routes.go 的 protocolV2Error → dispatcher 的 ErrDispatchTransport）。
// 因此错误分支同样走 TypedDocument，只是文档内容换成错误变体。
//
// schemaRef 决定 body 的形状：
//   - authorize.response  ：{"status":"error","error":…,"errorDescription":…}
//   - 其余（client-write / mutation / clients / authorizations …）：{"error":…,"errorDescription":…}
func routeFailure(
	ctx *protocolwire.RequestContext,
	status int,
	reason string,
	message string,
	schemaRef string,
) (*pluginwire.RouteResponse, error) {
	values := errorDocumentValues(reason, message, schemaRef)
	return routeDocument(ctx, status, schemaRef, values)
}

// errorDocumentValues 按目标 schema 生成错误文档。reason 形如 "oauth2.invalid_request"，
// 剥掉前缀后即为 RFC 6749 的 error code。
func errorDocumentValues(reason, message, schemaRef string) map[string]any {
	code := strings.TrimPrefix(reason, "oauth2.")
	if code == "" {
		code = "server_error"
	}
	if schemaRef == schemaAuthorizeResponse {
		return map[string]any{
			"status": "error", "error": code, "errorDescription": message,
		}
	}
	values := map[string]any{"error": code, "errorDescription": message}
	// client-write 与 mutation 的错误变体要求同时带 ok=false。
	if schemaRef == schemaClientWriteResponse || schemaRef == schemaMutationResponse {
		values["ok"] = false
	}
	return values
}

func responseContext(request *protocolwire.RequestContext) *protocolwire.ResponseContext {
	result := &protocolwire.ResponseContext{ServerTime: timestamppb.New(time.Now().UTC())}
	if request == nil {
		return result
	}
	result.RequestId = request.GetRequestId()
	if request.GetTrace() != nil {
		result.Trace = proto.Clone(request.GetTrace()).(*protocolwire.TraceContext)
	}
	if request.GetExtension() != nil {
		result.Extension = proto.Clone(request.GetExtension()).(*protocolwire.ExtensionIdentity)
	}
	return result
}

// ---------- 请求辅助 ----------

func actorUserID(request *pluginwire.RouteRequest) int64 {
	if request == nil || request.GetContext() == nil {
		return 0
	}
	return request.GetContext().GetActor().GetUserId()
}

// actorHasPermission 复核 Host guard 之外的权限键；Host guard 已经做过一次判断，
// 这里用于同一 handler 内区分“管理全部应用”和“仅管理自己的应用”。
func actorHasPermission(request *pluginwire.RouteRequest, keys ...string) bool {
	if request == nil || request.GetContext() == nil {
		return false
	}
	for _, held := range request.GetContext().GetActor().GetPermissionKeys() {
		if held == "*" {
			return true
		}
		for _, want := range keys {
			if held == want {
				return true
			}
		}
	}
	return false
}

func bodyValues(request *pluginwire.RouteRequest) map[string]any {
	if request == nil || request.GetBody() == nil || request.GetBody().GetValue() == nil {
		return nil
	}
	return request.GetBody().GetValue().AsMap()
}

func queryValue(request *pluginwire.RouteRequest, key string) string {
	if request == nil {
		return ""
	}
	return request.GetQueryParameters()[key]
}

func bodyString(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	text, _ := values[key].(string)
	return text
}

func bodyBool(values map[string]any, key string) bool {
	if values == nil {
		return false
	}
	switch typed := values[key].(type) {
	case bool:
		return typed
	case string:
		return typed == "true" || typed == "1"
	default:
		return false
	}
}

func bodyStringList(values map[string]any, key string) []string {
	if values == nil {
		return nil
	}
	raw, ok := values[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var _ = fmt.Sprintf

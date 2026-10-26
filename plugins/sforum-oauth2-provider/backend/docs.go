package main

import (
	"net/http"

	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
)

// 接入说明页的数据端点（GET /api/oauth/docs）。
//
// 页面本身是宿主贡献的公开页（theme.json → /docs/oauth2-provider，view: component）：
// 页头、页脚、站点布局与主题全部由 Core 渲染，正文由包内 L2 组件
// frontend/public/docs.mjs 挂载。插件不自己输出整页 HTML——那会绕开宿主的导航、
// 页脚、外观与 SEO 管道，视觉上也与站点割裂。
//
// 这里只提供**会漂移的那部分数据**：scope 清单、端点清单、错误码。三者在代码里
// 各有一份唯一事实源（scopeRegistry / 本文件的 docsEndpoints / tokenError+oauthError
// 的码字面量），散文与版式留在前端组件里。

// docsEndpoint 描述一个对外端点。Internal 为 true 的进「插件自用端点」折叠表：
// 这些路由由确认页与管理端调用，第三方应用无需接入，但清单里有就必须被文档覆盖
// （docs_test.go 要求 manifest routes[].path 全部出现在数据里）。
type docsEndpoint struct {
	Method   string
	Path     string
	Guard    string
	Note     string
	Internal bool
}

// docsErrorCode 是「错误码」表的行。码值必须真实存在于 token.go / authorize.go /
// userinfo.go 的错误分支里——测试会扫源码交叉验证，凭空编一个码会立刻变红。
type docsErrorCode struct {
	Code    string
	Meaning string
	When    string
}

// docsEndpoints 是端点总表的事实源。顺序即页面顺序：先第三方接入的六个端点，
// 再插件自用的内部端点。
var docsEndpoints = []docsEndpoint{
	{
		Method: "GET", Path: authorizeEndpointPath, Guard: "core.guard.public",
		Note: "授权入口。未登录先跳站内登录页；需要确认时跳确认页；已记住的授权直接回跳并带 code。",
	},
	{
		Method: "GET", Path: "/api/oauth/csrf", Guard: "core.guard.public",
		Note: "非浏览器客户端的 CSRF 握手：GET 会种下 csrf_ cookie，后续 unsafe 调用回传 X-Csrf-Token。",
	},
	{
		Method: "POST", Path: "/api/oauth/token", Guard: "core.guard.public",
		Note: "授权码兑换访问令牌；grant_type=refresh_token 时轮换刷新令牌。",
	},
	{
		Method: "GET", Path: "/api/oauth/userinfo", Guard: "core.guard.public",
		Note: "读取用户资料。访问令牌走 access_token 查询参数，不支持 Authorization: Bearer。",
	},
	{
		Method: "POST", Path: "/api/oauth/revoke", Guard: "core.guard.public",
		Note: "RFC 7009 撤销令牌：撤销刷新令牌会连带作废其访问令牌。",
	},
	{
		Method: "POST", Path: "/api/oauth/introspect", Guard: "core.guard.public",
		Note: "RFC 7662 令牌内省：返回 active 布尔与令牌元数据。",
	},

	{
		Method: "GET", Path: docsEndpointPath, Guard: "core.guard.public",
		Note: "本页正文的数据来源（scope / 端点 / 错误码），由页面组件读取。", Internal: true,
	},
	{
		Method: "GET", Path: consentEndpointPath, Guard: "core.guard.login",
		Note: "授权确认页（stream，自包含 HTML），带 request 与 return 参数。", Internal: true,
	},
	{
		Method: "GET", Path: "/api/oauth/consent/summary", Guard: "core.guard.login",
		Note: "确认页数据：应用名、请求的权限、当前用户。", Internal: true,
	},
	{
		Method: "POST", Path: "/api/oauth/consent/decision", Guard: "core.guard.login",
		Note: "允许 / 拒绝授权，返回回跳地址。", Internal: true,
	},
	{
		Method: "GET", Path: "/api/oauth/clients", Guard: "core.guard.login",
		Note: "应用列表：管理员看全部，普通用户看自己创建的。", Internal: true,
	},
	{
		Method: "POST", Path: "/api/oauth/clients/create", Guard: "core.guard.login",
		Note: "创建应用，响应里带回一次性的明文密钥。", Internal: true,
	},
	{
		Method: "POST", Path: "/api/oauth/clients/update", Guard: "core.guard.login",
		Note: "更新名称、说明、图标、回调地址与权限范围。", Internal: true,
	},
	{
		Method: "POST", Path: "/api/oauth/clients/rotate-secret", Guard: "core.guard.login",
		Note: "重置密钥，响应里带回一次性的新明文密钥。", Internal: true,
	},
	{
		Method: "POST", Path: "/api/oauth/clients/delete", Guard: "core.guard.login",
		Note: "删除应用，同时清理它的令牌与授权。", Internal: true,
	},
	{
		Method: "GET", Path: "/api/oauth/authorizations", Guard: "core.guard.login",
		Note: "我授权过的应用。", Internal: true,
	},
	{
		Method: "POST", Path: "/api/oauth/authorizations/revoke", Guard: "core.guard.login",
		Note: "撤销我对某个应用的授权，并作废其令牌。", Internal: true,
	},
}

// docsErrorCodes 是「错误码」表。语义与状态码对齐代码里的真实分支：
// authorize 侧按 RFC 6749 §4.1.2.1 回跳，token / userinfo 侧直接返回 JSON 错误体。
var docsErrorCodes = []docsErrorCode{
	{
		Code: "invalid_request", Meaning: "请求参数缺失、重复或非法",
		When: "任一端点；授权端点侧的错误会回跳到已登记的回调地址",
	},
	{
		Code: "invalid_client", Meaning: "client_id 未知，或 client_secret 不匹配",
		When: "token / revoke / introspect，HTTP 401",
	},
	{
		Code: "invalid_grant", Meaning: "授权码或刷新令牌无效、已过期、已被使用，或 PKCE 校验失败",
		When: "token",
	},
	{
		Code: "invalid_scope", Meaning: "请求的 scope 超出应用登记范围，或刷新时超出原始授权",
		When: "authorize / token",
	},
	{
		Code: "invalid_token", Meaning: "访问令牌无效或已过期",
		When: "userinfo，HTTP 401",
	},
	{
		Code: "access_denied", Meaning: "用户在确认页拒绝了这次授权",
		When: "回跳回调地址，带 error=access_denied",
	},
	{
		Code: "server_error", Meaning: "插件侧失败（存储、签发或宿主查询不可用）",
		When: "任一端点；可原样重试",
	},
}

// handleDocs 输出文档页需要的三张表。scope 目录复用 /clients 的 scopeCatalogFor：
// 同一份注册表，两处不可能漂移。
func (s *oauthServer) handleDocs(request *pluginwire.RouteRequest) (*pluginwire.RouteResponse, error) {
	return routeDocument(request.GetContext(), http.StatusOK, schemaDocsResponse, map[string]any{
		"ok":        true,
		"scopes":    scopeCatalogFor(),
		"endpoints": docsEndpointCatalog(),
		"errors":    docsErrorCatalog(),
	})
}

func docsEndpointCatalog() []any {
	rows := make([]any, 0, len(docsEndpoints))
	for _, item := range docsEndpoints {
		rows = append(rows, map[string]any{
			"method":   item.Method,
			"path":     item.Path,
			"guard":    item.Guard,
			"note":     item.Note,
			"internal": item.Internal,
		})
	}
	return rows
}

func docsErrorCatalog() []any {
	rows := make([]any, 0, len(docsErrorCodes))
	for _, item := range docsErrorCodes {
		rows = append(rows, map[string]any{
			"code":    item.Code,
			"meaning": item.Meaning,
			"when":    item.When,
		})
	}
	return rows
}

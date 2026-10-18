package main

// 路由 id 必须与 sforum.extension.json routes[].id 完全一致：Host 以 id 分发。
const (
	routeAuthorizeID         = "sforum.oauth2-provider.route.authorize"
	routeConsentPageID       = "sforum.oauth2-provider.route.consent-page"
	routeDocsID              = "sforum.oauth2-provider.route.docs"
	routeConsentSummaryID    = "sforum.oauth2-provider.route.consent-summary"
	routeConsentDecisionID   = "sforum.oauth2-provider.route.consent-decision"
	routeCSRFID              = "sforum.oauth2-provider.route.csrf"
	routeTokenID             = "sforum.oauth2-provider.route.token"
	routeUserinfoID          = "sforum.oauth2-provider.route.userinfo"
	routeRevokeID            = "sforum.oauth2-provider.route.revoke"
	routeIntrospectID        = "sforum.oauth2-provider.route.introspect"
	routeClientsListID       = "sforum.oauth2-provider.route.clients-list"
	routeClientsCreateID     = "sforum.oauth2-provider.route.clients-create"
	routeClientsUpdateID     = "sforum.oauth2-provider.route.clients-update"
	routeClientsDeleteID     = "sforum.oauth2-provider.route.clients-delete"
	routeClientsRotateID     = "sforum.oauth2-provider.route.clients-secret-rotate"
	routeAuthorizationsList  = "sforum.oauth2-provider.route.authorizations-list"
	routeAuthorizationsRevok = "sforum.oauth2-provider.route.authorizations-revoke"
)

// 插件自有端点路径。确认页的「切换账号」会把授权端点原样带回浏览器，
// 因此这里的常量同时是白名单校验（isAuthorizeReturnURL）的唯一事实源。
const (
	authorizeEndpointPath = "/api/oauth/authorize"
	consentEndpointPath   = "/api/oauth/consent"
	docsEndpointPath      = "/api/oauth/docs"
)

// 响应文档 schema（TypedDocument 的 schema 引用与 manifest 声明一致）。
const (
	schemaAuthorizeResponse       = "sforum.oauth2-provider.route.authorize.response@1"
	schemaConsentPageResponse     = "sforum.oauth2-provider.route.consent-page.response@1"
	schemaDocsResponse            = "sforum.oauth2-provider.route.docs.response@1"
	schemaConsentSummaryResponse  = "sforum.oauth2-provider.route.consent-summary.response@5"
	schemaConsentDecisionResponse = "sforum.oauth2-provider.route.consent-decision.response@1"
	schemaCSRFResponse            = "sforum.oauth2-provider.route.csrf.response@1"
	schemaTokenResponse           = "sforum.oauth2-provider.route.token.response@1"
	schemaUserinfoResponse        = "sforum.oauth2-provider.route.userinfo.response@4"
	schemaRevokeResponse          = "sforum.oauth2-provider.route.revoke.response@1"
	schemaIntrospectResponse      = "sforum.oauth2-provider.route.introspect.response@1"
	schemaClientsResponse         = "sforum.oauth2-provider.route.clients.response@7"
	schemaClientWriteResponse     = "sforum.oauth2-provider.route.client-write.response@5"
	schemaMutationResponse        = "sforum.oauth2-provider.route.mutation.response@1"
	schemaAuthorizationsResponse  = "sforum.oauth2-provider.route.authorizations.response@5"
)

// 宿主稳定查询（Stable HostQuery）合同：userinfo 的注册时间/资料更新时间。
// 这些 id 由宿主发布，插件只做消费；清单必须声明 `core_views` 授权，否则宿主返回
// host.query_core_views_denied（见 account.go 的降级路径）。
const (
	querySafeUserByID          = "sforum.core.safe_user.by_id"
	queryStableCorePlanVersion = "1"
	querySafeUserSchemaID      = "sforum.core.safe_user"
	querySafeUserSchemaVersion = "1"
	queryInt64ParameterRef     = "sforum.core.query.int64@1"
)

// 权限与令牌前缀。
const (
	permissionManageClients = "sforum.oauth2-provider.manage_clients"
	// permissionCreateClients 允许持权用户创建并管理自己创建的 OAuth2 应用（前台自助）。
	permissionCreateClients = "sforum.oauth2-provider.create_clients"

	prefixAuthorizationCode = "sfoc_"
	prefixAccessToken       = "sfoa_"
	prefixRefreshToken      = "sfor_"
	prefixClientSecret      = "sfos_"
	prefixConsentRequest    = "sfoq_"

	scopeProfile = "profile"

	// 可选 scope：每个都必须有真实的数据面支撑（见 userinfo.go）。
	// email 是敏感范围：授权时由宿主签发的一次性 query 委派读取本人邮箱并快照，
	// 之后 userinfo 只读快照（见 email_claim.go）。
	scopeEmail        = "email"
	scopeAvatar       = "avatar"
	scopeTopicCount   = "topic.count"
	scopeCommentCount = "comment.count"
	scopeUserGroups   = "users.groups"
)

// scopeDef 描述插件已实现的一个 OAuth2 scope。scopeRegistry 是插件侧的事实源：
// /clients 响应把它作为 scopeCatalog 下发，前端 chip 与授权确认页共用同一份定义。
type scopeDef struct {
	Name   string
	DescZh string
	DescEn string
	// LabelZh 是授权确认页上的一句话说明：比 DescZh 短，多个 scope 会被「、」
	// 拼成一行摘要，所以不带句末标点。新增 scope 时两处都要写。
	LabelZh  string
	AlwaysOn bool
}

// scopeRegistry 按展示顺序列出全部已实现的 scope。新增 scope 时在这里登记，
// 并同步 schemas/clients.response.json 的契约版本。
var scopeRegistry = []scopeDef{
	{
		Name:     scopeProfile,
		DescZh:   "读取你的基础资料：用户名、昵称、注册时间与账号状态。",
		DescEn:   "Read your basic profile: username, display name, registration date and account status.",
		LabelZh:  "读取你的用户名、昵称、注册时间与账号状态",
		AlwaysOn: true,
	},
	{
		Name:    scopeEmail,
		DescZh:  "读取你的邮箱地址与验证状态（授权时读取一次并保存，不会持续读取）。",
		DescEn:  "Read your email address and verification status (read once at authorization and stored).",
		LabelZh: "读取你的邮箱地址与验证状态",
	},
	{
		Name:    scopeAvatar,
		DescZh:  "读取你的头像地址（未上传头像时为按昵称生成的默认头像）。",
		DescEn:  "Read your avatar URL (a generated default avatar when you have not uploaded one).",
		LabelZh: "读取你的头像",
	},
	{
		Name:    scopeTopicCount,
		DescZh:  "读取你发布的主题数量。",
		DescEn:  "Read the number of topics you published.",
		LabelZh: "读取你发布的主题数量",
	},
	{
		Name:    scopeCommentCount,
		DescZh:  "读取你发布的回帖数量。",
		DescEn:  "Read the number of comments you posted.",
		LabelZh: "读取你发布的回帖数量",
	},
	{
		Name:    scopeUserGroups,
		DescZh:  "读取你的用户组标识（公开徽章数据）。",
		DescEn:  "Read your group badges (public membership data).",
		LabelZh: "读取你的用户组徽章",
	},
}

// scopeDefinition 按名称查注册表；未登记的 scope（历史令牌、手工写入的数据）
// 返回 false，由调用方回退到原始键名。
func scopeDefinition(name string) (scopeDef, bool) {
	for _, def := range scopeRegistry {
		if def.Name == name {
			return def, true
		}
	}
	return scopeDef{}, false
}

// scopeLabel 是授权确认页的权限说明。注册表是唯一事实源：新增 scope 只需在
// scopeRegistry 登记，确认页不会退化成裸键名。
func scopeLabel(scope string) string {
	if def, ok := scopeDefinition(scope); ok && def.LabelZh != "" {
		return def.LabelZh
	}
	return scope
}

// scopeCatalogFor 把注册表转成可下发的 JSON 目录（描述内联 i18n）。
// 返回 []any 以匹配 RPC 信封的 protobuf Value 编码（与 clients rows 同款）。
func scopeCatalogFor() []any {
	catalog := make([]any, 0, len(scopeRegistry))
	for _, def := range scopeRegistry {
		catalog = append(catalog, map[string]any{
			"name": def.Name,
			"description": map[string]any{
				"zh-CN": def.DescZh,
				"en-US": def.DescEn,
			},
			"alwaysOn": def.AlwaysOn,
		})
	}
	return catalog
}

// 设置键（由 manifest/settings.json 声明）。
const (
	settingAccessTTL  = "sforum.oauth2-provider.access_token_ttl"
	settingRefreshTTL = "sforum.oauth2-provider.refresh_token_ttl"
	settingCodeTTL    = "sforum.oauth2-provider.authorization_code_ttl"
	settingUserApps   = "sforum.oauth2-provider.allow_user_clients"
	settingConsent    = "sforum.oauth2-provider.require_consent"
	settingScopes     = "sforum.oauth2-provider.allowed_scopes"
)

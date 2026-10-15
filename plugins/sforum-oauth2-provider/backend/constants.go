package main

// 路由 id 必须与 sforum.extension.json routes[].id 完全一致：Host 以 id 分发。
const (
	routeAuthorizeID         = "sforum.oauth2-provider.route.authorize"
	routeConsentPageID       = "sforum.oauth2-provider.route.consent-page"
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

// 响应文档 schema（TypedDocument 的 schema 引用与 manifest 声明一致）。
const (
	schemaAuthorizeResponse       = "sforum.oauth2-provider.route.authorize.response@1"
	schemaConsentSummaryResponse  = "sforum.oauth2-provider.route.consent-summary.response@1"
	schemaConsentDecisionResponse = "sforum.oauth2-provider.route.consent-decision.response@1"
	schemaCSRFResponse            = "sforum.oauth2-provider.route.csrf.response@1"
	schemaTokenResponse           = "sforum.oauth2-provider.route.token.response@1"
	schemaUserinfoResponse        = "sforum.oauth2-provider.route.userinfo.response@1"
	schemaRevokeResponse          = "sforum.oauth2-provider.route.revoke.response@1"
	schemaIntrospectResponse      = "sforum.oauth2-provider.route.introspect.response@1"
	schemaClientsResponse         = "sforum.oauth2-provider.route.clients.response@1"
	schemaClientWriteResponse     = "sforum.oauth2-provider.route.client-write.response@1"
	schemaMutationResponse        = "sforum.oauth2-provider.route.mutation.response@1"
	schemaAuthorizationsResponse  = "sforum.oauth2-provider.route.authorizations.response@1"
)

// 权限与令牌前缀。
const (
	permissionManageClients = "sforum.oauth2-provider.manage_clients"

	prefixAuthorizationCode = "sfoc_"
	prefixAccessToken       = "sfoa_"
	prefixRefreshToken      = "sfor_"
	prefixClientSecret      = "sfos_"
	prefixConsentRequest    = "sfoq_"

	scopeProfile = "profile"
)

// 设置键（由 manifest/settings.json 声明）。
const (
	settingAccessTTL  = "sforum.oauth2-provider.access_token_ttl"
	settingRefreshTTL = "sforum.oauth2-provider.refresh_token_ttl"
	settingCodeTTL    = "sforum.oauth2-provider.authorization_code_ttl"
	settingUserApps   = "sforum.oauth2-provider.allow_user_clients"
	settingConsent    = "sforum.oauth2-provider.require_consent"
	settingScopes     = "sforum.oauth2-provider.allowed_scopes"
)

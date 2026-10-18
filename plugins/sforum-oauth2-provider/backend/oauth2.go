package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// OAuth2 引擎：令牌生成、PKCE 校验、redirect_uri 精确匹配、授权码与令牌签发。
// 所有持久化都只保存 token 的 SHA-256，明文仅在响应中返回一次。

var (
	errClientNotFound     = errors.New("oauth2: client not found")
	errInvalidSecret      = errors.New("oauth2: invalid client secret")
	errUnknownScope       = errors.New("oauth2: unknown scope")
	errScopeNotRegistered = errors.New("oauth2: scope is not registered for this client")
	errInvalidRedirectURI = errors.New("oauth2: redirect uri is not registered")
	errInvalidGrant       = errors.New("oauth2: invalid or expired grant")
	errPKCEFailed         = errors.New("oauth2: pkce verification failed")
	errPKCEMissing        = errors.New("oauth2: pkce code_verifier is required")
	errGrantTypeDisabled  = errors.New("oauth2: grant type is not enabled")
)

// generateToken 生成 `<prefix><32 字节随机 hex>` 形式的明文令牌。
func generateToken(prefix string) (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buffer), nil
}

// hashToken 是令牌的持久化形式；比较时使用常量时间。
func hashToken(plaintext string) string {
	digest := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(digest[:])
}

func hashesEqual(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

// normalizeScopes 把空格分隔的 scope 字符串规范为去重且稳定的列表。
// profile 是「用 SForum 登录」的隐含授予：无论请求了什么，身份读取始终在列
// 且稳定排在最前。这让 authorize 与 clients 两条路径共享同一条不变量。
func normalizeScopes(requested string, allowed []string) ([]string, error) {
	if len(allowed) == 0 {
		allowed = []string{scopeProfile}
	}
	permitted := make(map[string]struct{}, len(allowed))
	for _, scope := range allowed {
		permitted[scope] = struct{}{}
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, scope := range strings.Fields(requested) {
		if _, ok := permitted[scope]; !ok {
			return nil, fmt.Errorf("%w: %s", errUnknownScope, scope)
		}
		if _, duplicate := seen[scope]; duplicate {
			continue
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	ordered := make([]string, 0, len(result)+1)
	ordered = append(ordered, scopeProfile)
	for _, scope := range result {
		if scope != scopeProfile {
			ordered = append(ordered, scope)
		}
	}
	return ordered, nil
}

func joinScopes(scopes []string) string {
	return strings.Join(scopes, " ")
}

// resolveAuthorizeScopes 决定一次授权请求实际生效的 scope 集合，三层约束从内到外：
//
//  1. 请求未带 scope 时回落到应用登记的范围（RFC 6749 §3.3 允许服务端预定义默认值）。
//     应用在后台勾的权限因此就是「它默认会申请的权限」，而不是一张无人读取的配置。
//  2. 站点 allowed_scopes 是硬上限，由 normalizeScopes 负责（含 profile 必选）。
//  3. 应用登记的范围是该应用的自我约束：请求超出即 invalid_scope。没有这一层，
//     任何应用都能凭 client_id 直接索取站点开放的全部权限，后台的勾选形同虚设。
//
// profile 始终放行：它是 normalizeScopes 强制并入的身份基底，不能因为存量应用
// 的 scopes 列里没有它就把请求打回。
func resolveAuthorizeScopes(requested string, registered []string, allowed []string) ([]string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = joinScopes(registered)
	}
	scopes, err := normalizeScopes(requested, allowed)
	if err != nil {
		return nil, err
	}
	if len(registered) == 0 {
		return scopes, nil
	}
	permitted := make(map[string]struct{}, len(registered)+1)
	permitted[scopeProfile] = struct{}{}
	for _, scope := range registered {
		permitted[scope] = struct{}{}
	}
	for _, scope := range scopes {
		if _, ok := permitted[scope]; !ok {
			return nil, fmt.Errorf("%w: %s", errScopeNotRegistered, scope)
		}
	}
	return scopes, nil
}

// scopeSatisfies 判断已授权的 scope 是否覆盖本次请求（用于跳过重复确认）。
func scopeSatisfies(granted, requested string) bool {
	held := make(map[string]struct{})
	for _, scope := range strings.Fields(granted) {
		held[scope] = struct{}{}
	}
	for _, scope := range strings.Fields(requested) {
		if _, ok := held[scope]; !ok {
			return false
		}
	}
	return true
}

// matchRedirectURI 依据 RFC 6749 §3.1.2 做精确匹配（不做通配与路径前缀匹配）。
func matchRedirectURI(registered []string, candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && strings.HasPrefix(parsed.Host, "127.0.0.1")) &&
		!(parsed.Scheme == "http" && strings.HasPrefix(parsed.Host, "localhost")) {
		return false
	}
	for _, item := range registered {
		if item == candidate {
			return true
		}
	}
	return false
}

// validRedirectURIRegistration 在登记阶段做与运行时一致的校验。
func validRedirectURIRegistration(candidate string) bool {
	return matchRedirectURI([]string{candidate}, candidate)
}

// validLogoURL 校验应用 logo 地址：允许空（表示用生成 icon）；非空须 https
// （本地开发放行 http://127.0.0.1 / localhost），有 host、无 fragment、长度受限。
// 消费端会把它作为 <img src> 加载，因此约束与回调地址同源同风格。
func validLogoURL(candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return true
	}
	if len(candidate) > 2048 {
		return false
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" || parsed.Fragment != "" {
		return false
	}
	if parsed.Scheme == "https" {
		return true
	}
	return parsed.Scheme == "http" && (strings.HasPrefix(parsed.Host, "127.0.0.1") || strings.HasPrefix(parsed.Host, "localhost"))
}

// verifyPKCE 校验 RFC 7636 的 code_challenge，支持 S256 与 plain。
func verifyPKCE(challenge, method, verifier string) error {
	if challenge == "" {
		if verifier != "" {
			return nil
		}
		return nil
	}
	if verifier == "" {
		return errPKCEMissing
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		return errPKCEFailed
	}
	switch method {
	case "", "plain":
		if subtle.ConstantTimeCompare([]byte(verifier), []byte(challenge)) != 1 {
			return errPKCEFailed
		}
		return nil
	case "S256":
		// RFC 7636 §4.2：challenge = BASE64URL-ENCODE(SHA256(ASCII(verifier)))，无填充。
		digest := sha256.Sum256([]byte(verifier))
		computed := base64.RawURLEncoding.EncodeToString(digest[:])
		if subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) != 1 {
			return errPKCEFailed
		}
		return nil
	default:
		return errPKCEFailed
	}
}

// validCodeChallenge 校验授权请求携带的 PKCE 参数形态。
func validCodeChallenge(challenge, method string) bool {
	if challenge == "" {
		return method == ""
	}
	if len(challenge) < 43 || len(challenge) > 128 {
		return false
	}
	return method == "S256" || method == "plain"
}

// tokenPair 是一次成功签发的结果；明文只在此结构中短暂存在。
type tokenPair struct {
	AccessToken      string
	RefreshToken     string
	ExpiresIn        int64
	Scope            string
	AccessHash       string
	RefreshHash      string
	RefreshExpiresAt time.Time
}

// issueTokens 签发访问令牌（以及可选刷新令牌）。
func (s *oauthServer) issueTokens(
	ctx context.Context,
	clientID string,
	userID int64,
	scope string,
	parentRefreshHash string,
	withRefresh bool,
) (tokenPair, error) {
	settings := s.settings()
	accessToken, err := generateToken(prefixAccessToken)
	if err != nil {
		return tokenPair{}, err
	}
	now := time.Now().UTC()
	accessExpiry := now.Add(time.Duration(settings.AccessTokenTTL) * time.Second)
	pair := tokenPair{
		AccessToken: accessToken,
		ExpiresIn:   settings.AccessTokenTTL,
		Scope:       scope,
		AccessHash:  hashToken(accessToken),
	}
	if err := s.store.putToken(ctx, tokenRecord{
		TokenHash: pair.AccessHash, Kind: "access", ClientID: clientID, UserID: userID,
		Scope: scope, ParentHash: parentRefreshHash, ExpiresAt: accessExpiry.Format(timeLayout),
	}); err != nil {
		return tokenPair{}, err
	}
	if !withRefresh {
		return pair, nil
	}
	refreshToken, err := generateToken(prefixRefreshToken)
	if err != nil {
		return tokenPair{}, err
	}
	pair.RefreshToken = refreshToken
	pair.RefreshHash = hashToken(refreshToken)
	pair.RefreshExpiresAt = now.Add(time.Duration(settings.RefreshTokenTTL) * time.Second)
	if err := s.store.putToken(ctx, tokenRecord{
		TokenHash: pair.RefreshHash, Kind: "refresh", ClientID: clientID, UserID: userID,
		Scope: scope, ExpiresAt: pair.RefreshExpiresAt.Format(timeLayout),
	}); err != nil {
		return tokenPair{}, err
	}
	return pair, nil
}

// activeToken 读取并校验一个未撤销、未过期的令牌。
func (s *oauthServer) activeToken(ctx context.Context, plaintext, kind string) (tokenRecord, bool) {
	record, ok := s.store.getToken(ctx, hashToken(plaintext))
	if !ok || record.Kind != kind || record.RevokedAt != "" {
		return tokenRecord{}, false
	}
	expiry, err := time.Parse(timeLayout, record.ExpiresAt)
	if err != nil || !expiry.After(time.Now().UTC()) {
		return tokenRecord{}, false
	}
	return record, true
}

// authenticateClient 校验客户端身份。公开客户端必须依赖 PKCE，无需密钥。
func (s *oauthServer) authenticateClient(ctx context.Context, clientID, secret string) (clientRecord, error) {
	client, ok := s.store.getClient(ctx, clientID)
	if !ok {
		return clientRecord{}, errClientNotFound
	}
	if client.ClientType == "public" || client.SecretHash == "" {
		if secret != "" {
			return clientRecord{}, errInvalidSecret
		}
		return client, nil
	}
	if secret == "" {
		return clientRecord{}, errInvalidSecret
	}
	if !hashesEqual(hashToken(secret), client.SecretHash) {
		return clientRecord{}, errInvalidSecret
	}
	return client, nil
}

// newClientSecret 生成应用密钥与它的哈希。
func newClientSecret() (plaintext string, hash string, err error) {
	plaintext, err = generateToken(prefixClientSecret)
	if err != nil {
		return "", "", err
	}
	return plaintext, hashToken(plaintext), nil
}

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
)

// 应用管理：管理员可管理全部应用（sforum.oauth2-provider.manage_clients），
// 普通用户只能管理自己创建的应用与自己的授权记录。

func (s *oauthServer) handleClientsList(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	userID := actorUserID(request)
	canManageAll := actorHasPermission(request, permissionManageClients)
	owner := userID
	if canManageAll {
		owner = 0
	}
	records := s.store.listClients(ctx, owner)
	rows := make([]any, 0, len(records))
	for _, record := range records {
		rows = append(rows, clientToMap(record))
	}
	settings := s.settingsSnapshot(ctx, request.GetContext())
	return routeDocument(request.GetContext(), http.StatusOK, schemaClientsResponse, map[string]any{
		"clients":         rows,
		"canManageAll":    canManageAll,
		"allowUserClients": settings.AllowUserApps,
		"source":          s.store.sourceMode(),
	})
}

// canWriteClient 判断 actor 是否可以修改该应用。
func canWriteClient(request *pluginwire.RouteRequest, record clientRecord) bool {
	if actorHasPermission(request, permissionManageClients) {
		return true
	}
	return record.OwnerUserID > 0 && record.OwnerUserID == actorUserID(request)
}

func (s *oauthServer) handleClientCreate(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	values := bodyValues(request)
	userID := actorUserID(request)
	canManageAll := actorHasPermission(request, permissionManageClients)
	settings := s.settingsSnapshot(ctx, request.GetContext())
	if !settings.AllowUserApps && !canManageAll {
		return routeFailure(request.GetContext(), http.StatusForbidden, "oauth2.client_creation_disabled",
			"self-service app creation is disabled; ask an administrator to create the app"), nil
	}
	client, secret, err := s.buildClient(ctx, request, values, clientRecord{}, userID)
	if err != nil {
		return clientWriteFailure(request, err)
	}
	if err := s.store.putClient(ctx, client); err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.client_store_failed",
			"could not store the client"), nil
	}
	return clientWriteSuccess(request, client, secret)
}

func (s *oauthServer) handleClientUpdate(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	values := bodyValues(request)
	clientID := bodyString(values, "clientId")
	existing, ok := s.store.getClient(ctx, clientID)
	if !ok {
		return routeFailure(request.GetContext(), http.StatusNotFound, "oauth2.client_not_found", "unknown client"), nil
	}
	if !canWriteClient(request, existing) {
		return routeFailure(request.GetContext(), http.StatusForbidden, "oauth2.client_forbidden",
			"you may only manage applications you created"), nil
	}
	client, secret, err := s.buildClient(ctx, request, values, existing, existing.OwnerUserID)
	if err != nil {
		return clientWriteFailure(request, err)
	}
	if err := s.store.putClient(ctx, client); err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.client_store_failed",
			"could not store the client"), nil
	}
	// 更新不返回明文密钥；密钥只在创建或轮换时展示一次。
	return clientWriteSuccess(request, client, secret)
}

func (s *oauthServer) handleClientDelete(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	clientID := bodyString(bodyValues(request), "clientId")
	existing, ok := s.store.getClient(ctx, clientID)
	if !ok {
		return routeFailure(request.GetContext(), http.StatusNotFound, "oauth2.client_not_found", "unknown client"), nil
	}
	if !canWriteClient(request, existing) {
		return routeFailure(request.GetContext(), http.StatusForbidden, "oauth2.client_forbidden",
			"you may only manage applications you created"), nil
	}
	if err := s.store.deleteClient(ctx, clientID); err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.client_delete_failed",
			"could not delete the client"), nil
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaMutationResponse, map[string]any{"ok": true})
}

func (s *oauthServer) handleClientRotateSecret(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	clientID := bodyString(bodyValues(request), "clientId")
	existing, ok := s.store.getClient(ctx, clientID)
	if !ok {
		return routeFailure(request.GetContext(), http.StatusNotFound, "oauth2.client_not_found", "unknown client"), nil
	}
	if !canWriteClient(request, existing) {
		return routeFailure(request.GetContext(), http.StatusForbidden, "oauth2.client_forbidden",
			"you may only manage applications you created"), nil
	}
	secret, hash, err := newClientSecret()
	if err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.secret_generation_failed",
			"could not generate a client secret"), nil
	}
	if existing.ClientType == "public" {
		return routeFailure(request.GetContext(), http.StatusConflict, "oauth2.client_public",
			"public clients do not use a client secret"), nil
	}
	existing.SecretHash = hash
	if err := s.store.putClient(ctx, existing); err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.client_store_failed",
			"could not store the client"), nil
	}
	return clientWriteSuccess(request, existing, secret)
}

// handleAuthorizationsList 返回 actor 自己的授权记录。
func (s *oauthServer) handleAuthorizationsList(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	userID := actorUserID(request)
	records := s.store.listConsents(ctx, userID)
	rows := make([]any, 0, len(records))
	for _, record := range records {
		name := record.ClientID
		if client, ok := s.store.getClient(ctx, record.ClientID); ok {
			name = client.Name
		}
		rows = append(rows, map[string]any{
			"clientId":   record.ClientID,
			"clientName": name,
			"scopes":     splitScopes(record.Scope),
			"createdAt":  record.CreatedAt,
			"updatedAt":  record.UpdatedAt,
		})
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaAuthorizationsResponse, map[string]any{
		"authorizations": rows,
		"canManageAll":   actorHasPermission(request, permissionManageClients),
		"source":         s.store.sourceMode(),
	})
}

// handleAuthorizationRevoke 撤销用户对某个应用的授权，并作废其全部令牌。
func (s *oauthServer) handleAuthorizationRevoke(
	ctx context.Context,
	request *pluginwire.RouteRequest,
) (*pluginwire.RouteResponse, error) {
	clientID := bodyString(bodyValues(request), "clientId")
	userID := actorUserID(request)
	targetUser := userID
	if ownerID, ok := int64Value(bodyValues(request)["userId"]); ok && actorHasPermission(request, permissionManageClients) {
		targetUser = ownerID
	}
	revokedTokens, err := s.store.revokeClientTokens(ctx, targetUser, clientID)
	if err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.revoke_failed",
			"could not revoke tokens"), nil
	}
	removed, err := s.store.deleteConsent(ctx, targetUser, clientID)
	if err != nil {
		return routeFailure(request.GetContext(), http.StatusInternalServerError, "oauth2.revoke_failed",
			"could not remove the authorization"), nil
	}
	removedCount := 0
	if removed {
		removedCount = 1
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaMutationResponse, map[string]any{
		"ok":                     true,
		"revokedTokens":          revokedTokens,
		"revokedAuthorizations":  removedCount,
	})
}

// buildClient 校验并构造应用记录。existing 为空表示新建。
func (s *oauthServer) buildClient(
	ctx context.Context,
	request *pluginwire.RouteRequest,
	values map[string]any,
	existing clientRecord,
	ownerUserID int64,
) (clientRecord, string, error) {
	name := strings.TrimSpace(firstNonEmpty(bodyString(values, "name"), existing.Name))
	if name == "" || len(name) > 120 {
		return clientRecord{}, "", invalidRequest("name is required and must be at most 120 characters")
	}
	clientType := firstNonEmpty(bodyString(values, "clientType"), existing.ClientType, "confidential")
	if clientType != "confidential" && clientType != "public" {
		return clientRecord{}, "", invalidRequest("clientType must be confidential or public")
	}
	redirectURIs := bodyStringList(values, "redirectUris")
	if len(redirectURIs) == 0 {
		redirectURIs = existing.RedirectURIs
	}
	if len(redirectURIs) == 0 {
		return clientRecord{}, "", invalidRequest("at least one redirect URI is required")
	}
	if len(redirectURIs) > 10 {
		return clientRecord{}, "", invalidRequest("at most 10 redirect URIs are supported")
	}
	cleanURIs := make([]string, 0, len(redirectURIs))
	for _, candidate := range redirectURIs {
		candidate = strings.TrimSpace(candidate)
		if !validRedirectURIRegistration(candidate) {
			return clientRecord{}, "", invalidRequest("redirect URI must be https (http on loopback is allowed) without a fragment")
		}
		cleanURIs = append(cleanURIs, candidate)
	}
	settings := s.settingsSnapshot(ctx, request.GetContext())
	scopes := bodyStringList(values, "scopes")
	if len(scopes) == 0 {
		scopes = existing.Scopes
	}
	if len(scopes) == 0 {
		scopes = settings.AllowedScopes
	}
	normalized, err := normalizeScopes(joinScopes(scopes), settings.AllowedScopes)
	if err != nil {
		return clientRecord{}, "", invalidScope("requested scopes are not allowed")
	}

	record := existing
	record.Name = name
	record.Description = strings.TrimSpace(bodyString(values, "description"))
	if record.Description == "" && bodyString(values, "description") == "" {
		record.Description = existing.Description
	}
	record.ClientType = clientType
	record.RedirectURIs = cleanURIs
	record.Scopes = normalized
	record.OwnerUserID = ownerUserID
	record.OwnerName = existing.OwnerName
	if record.OwnerName == "" {
		if profile, ok := s.loadUserProfile(ctx, request.GetContext(), ownerUserID); ok {
			record.OwnerName = firstNonEmpty(profile.DisplayName, profile.Username)
		}
	}

	secret := ""
	if record.ClientID == "" {
		clientID, err := newClientID()
		if err != nil {
			return clientRecord{}, "", err
		}
		record.ClientID = clientID
		record.CreatedAt = time.Now().UTC().Format(timeLayout)
	}
	if clientType == "public" {
		record.SecretHash = ""
	} else if record.SecretHash == "" {
		generated, hash, err := newClientSecret()
		if err != nil {
			return clientRecord{}, "", err
		}
		secret = generated
		record.SecretHash = hash
	}
	return record, secret, nil
}

// newClientID 生成应用 id（不含密钥语义，可被第三方公开使用）。
func newClientID() (string, error) {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return "sfo_client_" + hex.EncodeToString(buffer), nil
}

func clientToMap(record clientRecord) map[string]any {
	return map[string]any{
		"clientId":     record.ClientID,
		"name":         record.Name,
		"description":  record.Description,
		"clientType":   record.ClientType,
		"redirectUris": splitScopes0(record.RedirectURIs),
		"scopes":       splitScopes(joinScopes(record.Scopes)),
		"ownerUserId":  record.OwnerUserID,
		"ownerName":    record.OwnerName,
		"createdAt":    record.CreatedAt,
		"lastUsedAt":   record.LastUsedAt,
		"hasSecret":    record.SecretHash != "",
	}
}

func clientWriteSuccess(request *pluginwire.RouteRequest, record clientRecord, secret string) (*pluginwire.RouteResponse, error) {
	values := map[string]any{"ok": true, "client": clientToMap(record)}
	if secret != "" {
		values["clientSecret"] = secret
	}
	return routeDocument(request.GetContext(), http.StatusOK, schemaClientWriteResponse, values)
}

func clientWriteFailure(request *pluginwire.RouteRequest, err error) (*pluginwire.RouteResponse, error) {
	status := http.StatusBadRequest
	reason := "oauth2.invalid_request"
	var oauthErr oauthError
	if errors.As(err, &oauthErr) {
		reason = "oauth2." + oauthErr.Code
	}
	return routeFailure(request.GetContext(), status, reason, err.Error()), nil
}

// splitScopes 把空格分隔的 scope 字符串转成数组（用于 JSON 响应）。
func splitScopes(value string) []any {
	result := make([]any, 0)
	for _, scope := range strings.Fields(value) {
		result = append(result, scope)
	}
	return result
}

func splitScopes0(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// oauthStore 持久化 OAuth2 应用、授权请求、令牌与授权记录。
// 优先使用 Host 授予的 own_schema 数据库（SFORUM_DATABASE_URL）；
// 没有数据库租约时回退到进程内存储，保证契约测试与本地演示可用。
type oauthStore struct {
	mu sync.Mutex

	db *sql.DB

	clients     map[string]clientRecord
	requests    map[string]requestRecord
	tokens      map[string]tokenRecord
	consents    map[string]consentRecord
	emailClaims map[string]emailClaimRecord
}

// clientRecord 是一条 OAuth2 应用记录。secret_hash 为空表示公开客户端。
type clientRecord struct {
	ClientID    string `json:"clientId"`
	SecretHash  string `json:"secretHash"`
	Name        string `json:"name"`
	Description string `json:"description"`
	ClientType  string `json:"clientType"`
	LogoURL     string `json:"logoUrl"`
	// AttachmentPublicID 是 Core 附件体系里的 publicId（上传的图标）。
	// 与 LogoURL 互斥：任何一次保存只保留一个，切换时旧的一侧必须被清理
	// （附件要走 release，否则引用永远挂着、图永不回收）。
	AttachmentPublicID string   `json:"attachmentPublicId"`
	RedirectURIs       []string `json:"redirectUris"`
	Scopes             []string `json:"scopes"`
	OwnerUserID        int64    `json:"ownerUserId"`
	OwnerName          string   `json:"ownerName"`
	CreatedAt          string   `json:"createdAt"`
	LastUsedAt         string   `json:"lastUsedAt"`
}

// requestRecord 是一次待确认的授权请求（授权码前置状态）。
type requestRecord struct {
	TokenHash           string `json:"tokenHash"`
	ClientID            string `json:"clientId"`
	UserID              int64  `json:"userId"`
	RedirectURI         string `json:"redirectUri"`
	Scope               string `json:"scope"`
	State               string `json:"state"`
	CodeChallenge       string `json:"codeChallenge"`
	CodeChallengeMethod string `json:"codeChallengeMethod"`
	ExpiresAt           string `json:"expiresAt"`
}

// tokenRecord 是授权码、访问令牌或刷新令牌。仅存哈希；
// 授权码额外保存回调地址与 PKCE 参数，供令牌端点复核。
type tokenRecord struct {
	TokenHash           string `json:"tokenHash"`
	Kind                string `json:"kind"`
	ClientID            string `json:"clientId"`
	UserID              int64  `json:"userId"`
	Scope               string `json:"scope"`
	ParentHash          string `json:"parentHash"`
	RedirectURI         string `json:"redirectUri"`
	CodeChallenge       string `json:"codeChallenge"`
	CodeChallengeMethod string `json:"codeChallengeMethod"`
	ExpiresAt           string `json:"expiresAt"`
	RevokedAt           string `json:"revokedAt"`
	CreatedAt           string `json:"createdAt"`
}

type consentRecord struct {
	UserID    int64  `json:"userId"`
	ClientID  string `json:"clientId"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// emailClaimRecord 是授权时快照下来的本人邮箱（scope=email）。
// 主键是 (user, client)：撤销该应用的授权、或新授权收窄到不含 email 时整行删除，
// 不做跨应用共享，避免「在 A 授权过，B 也能拿到」。
type emailClaimRecord struct {
	UserID        int64  `json:"userId"`
	ClientID      string `json:"clientId"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"emailVerified"`
	CapturedAt    string `json:"capturedAt"`
}

const timeLayout = time.RFC3339Nano

func newOAuthStore() *oauthStore {
	store := &oauthStore{
		clients:     map[string]clientRecord{},
		requests:    map[string]requestRecord{},
		tokens:      map[string]tokenRecord{},
		consents:    map[string]consentRecord{},
		emailClaims: map[string]emailClaimRecord{},
	}
	if url := strings.TrimSpace(os.Getenv("SFORUM_DATABASE_URL")); url != "" {
		if db, err := sql.Open("pgx", url); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if pingErr := db.PingContext(ctx); pingErr == nil {
				store.db = db
				_ = store.ensureSchema(ctx)
			} else {
				_ = db.Close()
			}
			cancel()
		}
	}
	return store
}

// sourceMode 供管理界面显示当前存储来源。
func (s *oauthStore) sourceMode() string {
	if s != nil && s.db != nil {
		return "database"
	}
	return "memory"
}

func (s *oauthStore) ensureSchema(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS schema_meta (
  key text PRIMARY KEY,
  value text NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS oauth_clients (
  client_id text PRIMARY KEY,
  secret_hash text NOT NULL DEFAULT '',
  name text NOT NULL,
  description text NOT NULL DEFAULT '',
  client_type text NOT NULL DEFAULT 'confidential',
  logo_url text NOT NULL DEFAULT '',
  attachment_public_id text NOT NULL DEFAULT '',
  redirect_uris text NOT NULL DEFAULT '',
  scopes text NOT NULL DEFAULT '',
  owner_user_id bigint NOT NULL DEFAULT 0,
  owner_name text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz
)`,
		`CREATE TABLE IF NOT EXISTS oauth_requests (
  token_hash text PRIMARY KEY,
  client_id text NOT NULL,
  user_id bigint NOT NULL DEFAULT 0,
  redirect_uri text NOT NULL,
  scope text NOT NULL DEFAULT '',
  state text NOT NULL DEFAULT '',
  code_challenge text NOT NULL DEFAULT '',
  code_challenge_method text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL
)`,
		`CREATE TABLE IF NOT EXISTS oauth_tokens (
  token_hash text PRIMARY KEY,
  kind text NOT NULL,
  client_id text NOT NULL,
  user_id bigint NOT NULL DEFAULT 0,
  scope text NOT NULL DEFAULT '',
  parent_hash text NOT NULL DEFAULT '',
  redirect_uri text NOT NULL DEFAULT '',
  code_challenge text NOT NULL DEFAULT '',
  code_challenge_method text NOT NULL DEFAULT '',
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
)`,
		`CREATE TABLE IF NOT EXISTS oauth_consents (
  user_id bigint NOT NULL,
  client_id text NOT NULL,
  scope text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, client_id)
)`,
		`CREATE TABLE IF NOT EXISTS oauth_email_claims (
  user_id bigint NOT NULL,
  client_id text NOT NULL,
  email text NOT NULL,
  email_verified boolean NOT NULL DEFAULT false,
  captured_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, client_id)
)`,
		`CREATE INDEX IF NOT EXISTS oauth_tokens_client_idx ON oauth_tokens (client_id)`,
		// 已部署库的幂等迁移：logo_url / attachment_public_id 都是后加的列，旧库没有它们。
		`ALTER TABLE oauth_clients ADD COLUMN IF NOT EXISTS logo_url text NOT NULL DEFAULT ''`,
		`ALTER TABLE oauth_clients ADD COLUMN IF NOT EXISTS attachment_public_id text NOT NULL DEFAULT ''`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO schema_meta (key, value) VALUES ('version', '1')
ON CONFLICT (key) DO NOTHING`)
	return nil
}

// ---------- 应用（clients） ----------

func (s *oauthStore) putClient(ctx context.Context, record clientRecord) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		s.clients[record.ClientID] = record
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_clients
  (client_id, secret_hash, name, description, client_type, logo_url, attachment_public_id, redirect_uris, scopes, owner_user_id, owner_name, created_at, last_used_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULL)
ON CONFLICT (client_id) DO UPDATE SET
  secret_hash = EXCLUDED.secret_hash, name = EXCLUDED.name, description = EXCLUDED.description,
  client_type = EXCLUDED.client_type, logo_url = EXCLUDED.logo_url,
  attachment_public_id = EXCLUDED.attachment_public_id,
  redirect_uris = EXCLUDED.redirect_uris, scopes = EXCLUDED.scopes,
  owner_user_id = EXCLUDED.owner_user_id, owner_name = EXCLUDED.owner_name`,
		record.ClientID, record.SecretHash, record.Name, record.Description, record.ClientType,
		record.LogoURL, record.AttachmentPublicID,
		strings.Join(record.RedirectURIs, "\n"), strings.Join(record.Scopes, " "),
		record.OwnerUserID, record.OwnerName, parseTimeOrNow(record.CreatedAt))
	return err
}

func (s *oauthStore) getClient(ctx context.Context, clientID string) (clientRecord, bool) {
	if s == nil {
		return clientRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		record, ok := s.clients[clientID]
		return record, ok
	}
	row := s.db.QueryRowContext(ctx, `SELECT client_id, secret_hash, name, description, client_type,
  logo_url, attachment_public_id, redirect_uris, scopes, owner_user_id, owner_name, created_at, last_used_at
FROM oauth_clients WHERE client_id = $1`, clientID)
	record, err := scanClient(row)
	if err != nil {
		return clientRecord{}, false
	}
	return record, true
}

func (s *oauthStore) listClients(ctx context.Context, ownerUserID int64) []clientRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		result := make([]clientRecord, 0, len(s.clients))
		for _, record := range s.clients {
			if ownerUserID > 0 && record.OwnerUserID != ownerUserID {
				continue
			}
			result = append(result, record)
		}
		sortClients(result)
		return result
	}
	query := `SELECT client_id, secret_hash, name, description, client_type, logo_url, attachment_public_id,
  redirect_uris, scopes, owner_user_id, owner_name, created_at, last_used_at FROM oauth_clients`
	args := []any{}
	if ownerUserID > 0 {
		query += ` WHERE owner_user_id = $1`
		args = append(args, ownerUserID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make([]clientRecord, 0)
	for rows.Next() {
		record, scanErr := scanClient(rows)
		if scanErr != nil {
			continue
		}
		result = append(result, record)
	}
	return result
}

func (s *oauthStore) deleteClient(ctx context.Context, clientID string) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		delete(s.clients, clientID)
		for key, record := range s.tokens {
			if record.ClientID == clientID {
				delete(s.tokens, key)
			}
		}
		for key, record := range s.consents {
			if record.ClientID == clientID {
				delete(s.consents, key)
			}
		}
		for key, record := range s.requests {
			if record.ClientID == clientID {
				delete(s.requests, key)
			}
		}
		for key, record := range s.emailClaims {
			if record.ClientID == clientID {
				delete(s.emailClaims, key)
			}
		}
		return nil
	}
	for _, statement := range []string{
		`DELETE FROM oauth_consents WHERE client_id = $1`,
		`DELETE FROM oauth_email_claims WHERE client_id = $1`,
		`DELETE FROM oauth_tokens WHERE client_id = $1`,
		`DELETE FROM oauth_requests WHERE client_id = $1`,
		`DELETE FROM oauth_clients WHERE client_id = $1`,
	} {
		if _, err := s.db.ExecContext(ctx, statement, clientID); err != nil {
			return err
		}
	}
	return nil
}

func (s *oauthStore) touchClient(ctx context.Context, clientID string) {
	s.mu.Lock()
	record, ok := s.clients[clientID]
	if !ok && s.db == nil {
		s.mu.Unlock()
		return
	}
	if s.db == nil {
		record.LastUsedAt = time.Now().UTC().Format(timeLayout)
		s.clients[clientID] = record
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	_, _ = s.db.ExecContext(ctx, `UPDATE oauth_clients SET last_used_at = now() WHERE client_id = $1`, clientID)
}

// ---------- 授权请求 ----------

func (s *oauthStore) putRequest(ctx context.Context, record requestRecord) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		s.requests[record.TokenHash] = record
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_requests
  (token_hash, client_id, user_id, redirect_uri, scope, state, code_challenge, code_challenge_method, expires_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (token_hash) DO UPDATE SET client_id = EXCLUDED.client_id, user_id = EXCLUDED.user_id,
  redirect_uri = EXCLUDED.redirect_uri, scope = EXCLUDED.scope, state = EXCLUDED.state,
  code_challenge = EXCLUDED.code_challenge, code_challenge_method = EXCLUDED.code_challenge_method,
  expires_at = EXCLUDED.expires_at`,
		record.TokenHash, record.ClientID, record.UserID, record.RedirectURI, record.Scope, record.State,
		record.CodeChallenge, record.CodeChallengeMethod, parseTimeOrNow(record.ExpiresAt))
	return err
}

func (s *oauthStore) getRequest(ctx context.Context, tokenHash string) (requestRecord, bool) {
	if s == nil {
		return requestRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		record, ok := s.requests[tokenHash]
		return record, ok
	}
	row := s.db.QueryRowContext(ctx, `SELECT token_hash, client_id, user_id, redirect_uri, scope, state,
  code_challenge, code_challenge_method, expires_at FROM oauth_requests WHERE token_hash = $1`, tokenHash)
	var record requestRecord
	var expires time.Time
	if err := row.Scan(&record.TokenHash, &record.ClientID, &record.UserID, &record.RedirectURI, &record.Scope,
		&record.State, &record.CodeChallenge, &record.CodeChallengeMethod, &expires); err != nil {
		return requestRecord{}, false
	}
	record.ExpiresAt = expires.UTC().Format(timeLayout)
	return record, true
}

func (s *oauthStore) deleteRequest(ctx context.Context, tokenHash string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		delete(s.requests, tokenHash)
		return
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM oauth_requests WHERE token_hash = $1`, tokenHash)
}

// ---------- 令牌 ----------

func (s *oauthStore) putToken(ctx context.Context, record tokenRecord) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		s.tokens[record.TokenHash] = record
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_tokens
  (token_hash, kind, client_id, user_id, scope, parent_hash, redirect_uri, code_challenge, code_challenge_method, expires_at, revoked_at, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULL,now())`,
		record.TokenHash, record.Kind, record.ClientID, record.UserID, record.Scope, record.ParentHash,
		record.RedirectURI, record.CodeChallenge, record.CodeChallengeMethod,
		parseTimeOrNow(record.ExpiresAt))
	return err
}

func (s *oauthStore) getToken(ctx context.Context, tokenHash string) (tokenRecord, bool) {
	if s == nil {
		return tokenRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		record, ok := s.tokens[tokenHash]
		return record, ok
	}
	row := s.db.QueryRowContext(ctx, `SELECT token_hash, kind, client_id, user_id, scope, parent_hash,
  redirect_uri, code_challenge, code_challenge_method, expires_at, revoked_at
FROM oauth_tokens WHERE token_hash = $1`, tokenHash)
	var record tokenRecord
	var expires time.Time
	var revoked sql.NullTime
	if err := row.Scan(&record.TokenHash, &record.Kind, &record.ClientID, &record.UserID, &record.Scope,
		&record.ParentHash, &record.RedirectURI, &record.CodeChallenge, &record.CodeChallengeMethod,
		&expires, &revoked); err != nil {
		return tokenRecord{}, false
	}
	record.ExpiresAt = expires.UTC().Format(timeLayout)
	if revoked.Valid {
		record.RevokedAt = revoked.Time.UTC().Format(timeLayout)
	}
	return record, true
}

func (s *oauthStore) revokeToken(ctx context.Context, tokenHash string) (bool, error) {
	if s == nil {
		return false, errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		record, ok := s.tokens[tokenHash]
		if !ok {
			return false, nil
		}
		if record.RevokedAt != "" {
			return false, nil
		}
		record.RevokedAt = time.Now().UTC().Format(timeLayout)
		s.tokens[tokenHash] = record
		return true, nil
	}
	result, err := s.db.ExecContext(ctx, `UPDATE oauth_tokens SET revoked_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// revokeClientTokens 撤销某个用户对某应用的全部令牌；clientID 为空表示该用户全部应用。
func (s *oauthStore) revokeClientTokens(ctx context.Context, userID int64, clientID string) (int, error) {
	if s == nil {
		return 0, errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(timeLayout)
	if s.db == nil {
		count := 0
		for key, record := range s.tokens {
			if record.UserID != userID || record.RevokedAt != "" {
				continue
			}
			if clientID != "" && record.ClientID != clientID {
				continue
			}
			record.RevokedAt = now
			s.tokens[key] = record
			count++
		}
		return count, nil
	}
	var result sql.Result
	var err error
	if clientID == "" {
		result, err = s.db.ExecContext(ctx, `UPDATE oauth_tokens SET revoked_at = now()
WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	} else {
		result, err = s.db.ExecContext(ctx, `UPDATE oauth_tokens SET revoked_at = now()
WHERE user_id = $1 AND client_id = $2 AND revoked_at IS NULL`, userID, clientID)
	}
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

// touchRefreshToken 记录刷新令牌最近一次使用时间（created_at 复用为 last used）。
func (s *oauthStore) markRefreshUsed(ctx context.Context, tokenHash string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE oauth_tokens SET created_at = now() WHERE token_hash = $1`, tokenHash)
}

// ---------- 邮箱快照（email claims） ----------

func (s *oauthStore) putEmailClaim(ctx context.Context, record emailClaimRecord) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		if record.CapturedAt == "" {
			record.CapturedAt = time.Now().UTC().Format(timeLayout)
		}
		s.emailClaims[consentKey(record.UserID, record.ClientID)] = record
		return nil
	}
	capturedAt := time.Now().UTC()
	if record.CapturedAt != "" {
		if parsed, err := time.Parse(timeLayout, record.CapturedAt); err == nil {
			capturedAt = parsed.UTC()
		}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_email_claims
  (user_id, client_id, email, email_verified, captured_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id, client_id) DO UPDATE SET
  email = EXCLUDED.email,
  email_verified = EXCLUDED.email_verified,
  captured_at = EXCLUDED.captured_at`,
		record.UserID, record.ClientID, record.Email, record.EmailVerified, capturedAt)
	return err
}

func (s *oauthStore) getEmailClaim(ctx context.Context, userID int64, clientID string) (emailClaimRecord, bool) {
	if s == nil {
		return emailClaimRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		record, ok := s.emailClaims[consentKey(userID, clientID)]
		return record, ok
	}
	var record emailClaimRecord
	var capturedAt time.Time
	err := s.db.QueryRowContext(ctx, `SELECT user_id, client_id, email, email_verified, captured_at
FROM oauth_email_claims WHERE user_id = $1 AND client_id = $2`, userID, clientID).Scan(
		&record.UserID, &record.ClientID, &record.Email, &record.EmailVerified, &capturedAt)
	if err != nil {
		return emailClaimRecord{}, false
	}
	record.CapturedAt = capturedAt.UTC().Format(timeLayout)
	return record, true
}

func (s *oauthStore) deleteEmailClaim(ctx context.Context, userID int64, clientID string) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		delete(s.emailClaims, consentKey(userID, clientID))
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM oauth_email_claims WHERE user_id = $1 AND client_id = $2`,
		userID, clientID)
	return err
}

// ---------- 授权记录（consents） ----------

func (s *oauthStore) upsertConsent(ctx context.Context, userID int64, clientID, scope string) error {
	if s == nil {
		return errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC().Format(timeLayout)
	if s.db == nil {
		key := consentKey(userID, clientID)
		record, ok := s.consents[key]
		if !ok {
			record = consentRecord{UserID: userID, ClientID: clientID, CreatedAt: now}
		}
		record.Scope = scope
		record.UpdatedAt = now
		s.consents[key] = record
		return nil
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO oauth_consents (user_id, client_id, scope, created_at, updated_at)
VALUES ($1,$2,$3,now(),now())
ON CONFLICT (user_id, client_id) DO UPDATE SET scope = EXCLUDED.scope, updated_at = now()`,
		userID, clientID, scope)
	return err
}

func (s *oauthStore) getConsent(ctx context.Context, userID int64, clientID string) (consentRecord, bool) {
	if s == nil {
		return consentRecord{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		record, ok := s.consents[consentKey(userID, clientID)]
		return record, ok
	}
	row := s.db.QueryRowContext(ctx, `SELECT scope, created_at, updated_at FROM oauth_consents
WHERE user_id = $1 AND client_id = $2`, userID, clientID)
	var scope string
	var created, updated time.Time
	if err := row.Scan(&scope, &created, &updated); err != nil {
		return consentRecord{}, false
	}
	return consentRecord{
		UserID: userID, ClientID: clientID, Scope: scope,
		CreatedAt: created.UTC().Format(timeLayout), UpdatedAt: updated.UTC().Format(timeLayout),
	}, true
}

func (s *oauthStore) listConsents(ctx context.Context, userID int64) []consentRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		result := make([]consentRecord, 0)
		for _, record := range s.consents {
			if record.UserID == userID {
				result = append(result, record)
			}
		}
		return result
	}
	rows, err := s.db.QueryContext(ctx, `SELECT client_id, scope, created_at, updated_at FROM oauth_consents
WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	result := make([]consentRecord, 0)
	for rows.Next() {
		var clientID, scope string
		var created, updated time.Time
		if err := rows.Scan(&clientID, &scope, &created, &updated); err != nil {
			continue
		}
		result = append(result, consentRecord{
			UserID: userID, ClientID: clientID, Scope: scope,
			CreatedAt: created.UTC().Format(timeLayout), UpdatedAt: updated.UTC().Format(timeLayout),
		})
	}
	return result
}

func (s *oauthStore) deleteConsent(ctx context.Context, userID int64, clientID string) (bool, error) {
	if s == nil {
		return false, errors.New("store unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		key := consentKey(userID, clientID)
		if _, ok := s.consents[key]; !ok {
			return false, nil
		}
		delete(s.consents, key)
		return true, nil
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM oauth_consents WHERE user_id = $1 AND client_id = $2`,
		userID, clientID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

func consentKey(userID int64, clientID string) string {
	return fmt.Sprintf("%d\x00%s", userID, clientID)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanClient(row rowScanner) (clientRecord, error) {
	var record clientRecord
	var redirectURIs, scopes string
	var createdAt time.Time
	var lastUsed sql.NullTime
	if err := row.Scan(&record.ClientID, &record.SecretHash, &record.Name, &record.Description,
		&record.ClientType, &record.LogoURL, &record.AttachmentPublicID, &redirectURIs, &scopes,
		&record.OwnerUserID, &record.OwnerName, &createdAt, &lastUsed); err != nil {
		return clientRecord{}, err
	}
	record.RedirectURIs = splitList(redirectURIs, "\n")
	record.Scopes = strings.Fields(scopes)
	record.CreatedAt = createdAt.UTC().Format(timeLayout)
	if lastUsed.Valid {
		record.LastUsedAt = lastUsed.Time.UTC().Format(timeLayout)
	}
	return record, nil
}

func splitList(value, separator string) []string {
	result := make([]string, 0)
	for _, item := range strings.Split(value, separator) {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseTimeOrNow(value string) time.Time {
	if parsed, err := time.Parse(timeLayout, value); err == nil {
		return parsed
	}
	return time.Now().UTC()
}

func sortClients(records []clientRecord) {
	for i := 1; i < len(records); i++ {
		for j := i; j > 0 && records[j].CreatedAt > records[j-1].CreatedAt; j-- {
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
}

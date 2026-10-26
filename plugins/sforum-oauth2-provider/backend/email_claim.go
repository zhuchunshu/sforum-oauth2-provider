package main

import (
	"context"
	"log"
	"strings"
	"time"

	protocolwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// 授权时的本人邮箱快照（scope=email）。
//
// 数据面：宿主在「宿主发起的 route/admin 调用」里签发一次性 query actor 委派，
// 插件凭委派读 `core.query.identity.self_email`。请求里没有任何 userId 参数，
// 因此只可能读到调用者本人；清单必须声明 `users.email.read`，否则宿主不会签发委派。
//
// 为什么要快照：委派是单次、≤2 分钟的，userinfo 调用（服务端到服务端、没有用户会话）
// 拿不到委派。所以在授权码签发时读一次、存进插件自己的 schema，userinfo 只读快照。
// 快照按 (user, client) 存：撤销该应用授权、或新授权不再包含 email 时整行删除。
//
// 失败一律降级：读不到就不写快照（userinfo 缺字段），绝不影响授权本身。
const (
	emailClaimTimeout = 3 * time.Second
)

// captureEmailClaim 在签发授权码之前同步邮箱快照。
// 授予 email → 读一次并落库；未授予 → 删除该 (user, client) 的旧快照。
func (s *oauthServer) captureEmailClaim(
	ctx context.Context,
	hostCtx *protocolwire.RequestContext,
	userID int64,
	clientID string,
	scope string,
) {
	if userID <= 0 || clientID == "" {
		return
	}
	if !scopeSatisfies(scope, scopeEmail) {
		// 新授权收窄到不含 email：旧快照必须跟着消失，否则会越过本次授权继续下发。
		if err := s.store.deleteEmailClaim(ctx, userID, clientID); err != nil {
			s.recordEmailClaimError("delete claim: " + err.Error())
		}
		return
	}
	claim, ok := s.readOwnEmail(ctx, hostCtx)
	if !ok {
		return
	}
	claim.UserID = userID
	claim.ClientID = clientID
	if err := s.store.putEmailClaim(ctx, claim); err != nil {
		s.recordEmailClaimError("store claim: " + err.Error())
		return
	}
}

// readOwnEmail 用宿主签发的一次性委派读取本人邮箱。
// 返回的 record 只带 email/emailVerified/capturedAt；user/client 由调用方补。
func (s *oauthServer) readOwnEmail(
	ctx context.Context,
	hostCtx *protocolwire.RequestContext,
) (emailClaimRecord, bool) {
	host, err := s.hostClient()
	if err != nil || host == nil || host.Queries == nil {
		return emailClaimRecord{}, false
	}
	// SelfEmailQueryRequest 会从 incoming context 里挑出唯一匹配的委派令牌，
	// 并把 Actor 与 Host*-Delegations 从回调上下文里清掉（宿主拒绝回传委派）。
	request, err := host.SelfEmailQueryRequest(hostCtx)
	if err != nil {
		// 没有委派：匿名调用、job/schedule 调用、或清单没声明 users.email.read。
		s.recordEmailClaimError("delegation unavailable: " + err.Error())
		return emailClaimRecord{}, false
	}
	queryCtx, cancel := context.WithTimeout(ctx, emailClaimTimeout)
	defer cancel()
	response, err := host.Queries.Execute(queryCtx, request)
	if err != nil || response == nil {
		s.recordEmailClaimError(firstNonEmpty(errorText(err), "self-email query returned no response"))
		return emailClaimRecord{}, false
	}
	if detail := response.GetError(); detail != nil {
		s.recordEmailClaimError(detail.GetReason() + ": " + detail.GetMessage())
		return emailClaimRecord{}, false
	}
	if len(response.GetRows()) == 0 {
		// 账号没有邮箱或不是 active：宿主返回空行集，这不是错误。
		return emailClaimRecord{}, false
	}
	values := response.GetRows()[0].GetValue().AsMap()
	email := strings.TrimSpace(stringValue(values["email"]))
	if email == "" {
		return emailClaimRecord{}, false
	}
	verified, _ := values["email_verified"].(bool)
	return emailClaimRecord{
		Email:         email,
		EmailVerified: verified,
		CapturedAt:    time.Now().UTC().Format(timeLayout),
	}, true
}

// recordEmailClaimError 保存最近一次快照失败原因，只在原因变化时打日志：
// 稳定失败（例如能力未确认）不应该把日志刷满，也不应该让第三方看到内部原因。
func (s *oauthServer) recordEmailClaimError(text string) {
	text = strings.TrimSpace(text)
	s.emailClaimErrorMu.Lock()
	previous := s.emailClaimError
	s.emailClaimError = text
	s.emailClaimErrorMu.Unlock()
	if text != "" && text != previous {
		log.Printf("oauth2-provider: email claim unavailable: %s", text)
	}
}

func (s *oauthServer) lastEmailClaimError() string {
	s.emailClaimErrorMu.RLock()
	defer s.emailClaimErrorMu.RUnlock()
	return s.emailClaimError
}

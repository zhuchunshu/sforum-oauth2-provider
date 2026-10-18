package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	pluginv2 "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2"
	hostwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/host/v2"
	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
	protocolwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/protocol/v2"
)

// 应用图标的「上传」通道：浏览器把图片直接 POST 到 Core 的
// /api/v1/attachments，插件只拿 publicId 调
// sforum.attachments.extension.claim@1 把它登记成「本扩展拥有的公开资产」。
//
// 插件自己的库只存 publicId，不存字节、不存 URL、不自建上传路由——对象存储、
// 出图、变体、引用计数与孤儿回收全部由 Core 持有。
//
// 两条命令都是 delegated actor 模式：Host 只在路由请求带 Idempotency-Key 时
// 才按 RequiredPermissions（attachment.upload）批量签发委派，插件用
// host.DelegatedCommandRequest 挑出与命令匹配的那一条。旧宿主 / 缺
// host_commands grant / 账号没有 attachment.upload 都表现为没有委派，统一返回
// ErrHostActorDelegationUnavailable，映射成可读的 icon_upload_unsupported。
const (
	attachmentClaimCommandID     = "sforum.attachments.extension.claim"
	attachmentReleaseCommandID   = "sforum.attachments.extension.release"
	attachmentCommandVersion     = "1"
	attachmentClaimInputSchema   = "sforum.attachments.extension.claim.input@1"
	attachmentReleaseInputSchema = "sforum.attachments.extension.release.input@1"

	// attachmentContextPrefix 让每个应用在扩展内有一个稳定的 claim 命名空间：
	// 同一 context 再 claim 会「删旧插新」（换图），release 也按它定位。
	attachmentContextPrefix = "oauth2-client:"

	// attachmentContextMaxLength 是 Host 对 context 的长度上限。
	attachmentContextMaxLength = 64
)

// claimedIcon 是 claim 成功后的结果。Revision 是 Host 返回的附件修订号，
// 只在 claim 请求信封里用得上（expectedRevision），不落库。
type claimedIcon struct {
	PublicID string
	URL      string
	Revision string
}

// attachmentContext 返回某个应用在扩展内的 claim context。
func attachmentContext(clientID string) string {
	context := attachmentContextPrefix + strings.TrimSpace(clientID)
	if len(context) > attachmentContextMaxLength {
		return context[:attachmentContextMaxLength]
	}
	return context
}

// iconError 是图标通道的稳定错误：Code 会被 routeFailure 拼成
// "oauth2.<Code>"，前端只认这个码，不解析 Host 文案。
func iconError(code, description string, status int) oauthError {
	return oauthError{Code: code, Description: description, Status: status}
}

// errIconUploadUnsupported 覆盖三种真实原因（旧宿主没有这三条命令、当前精确制品
// 没被授予 host_commands、账号没有 attachment.upload）。前端据此展开
// 「或使用图片地址」，绝不静默退回 base64 内联。
var errIconUploadUnsupported = iconError("icon_upload_unsupported",
	"icon upload is unavailable on this host version or for this account; upgrade SForum or use an image URL",
	http.StatusConflict)

// claimIcon 把已上传的附件登记成该应用的公开图标。
//
// revision 必须等于附件当前的 updated_at：浏览器上传响应里的 createdAt 与
// updated_at 是同一条 INSERT 里的两个 DEFAULT now()，取值相同，因此首次 claim
// 用它即可；被别的写路径推进过就返回 host.attachment_revision_conflict，
// 映射成「图片已被修改，请重新上传」。
func (s *oauthServer) claimIcon(
	ctx context.Context,
	parent *protocolwire.RequestContext,
	clientID string,
	publicID string,
	revision string,
) (claimedIcon, error) {
	publicID = strings.TrimSpace(publicID)
	if publicID == "" {
		return claimedIcon{}, iconError("icon_attachment_missing",
			"attachmentPublicId is required when logoUrl is empty", http.StatusBadRequest)
	}
	host, err := s.hostClient()
	if err != nil || host == nil {
		return claimedIcon{}, errIconUploadUnsupported
	}
	input, err := pluginv2.NewTypedDocument(attachmentClaimInputSchema, map[string]any{
		"attachmentPublicId": publicID,
		"context":            attachmentContext(clientID),
	})
	if err != nil {
		return claimedIcon{}, iconError("icon_upload_failed",
			"could not encode the claim request", http.StatusInternalServerError)
	}
	request, err := host.DelegatedCommandRequest(parent, attachmentClaimCommandID, attachmentCommandVersion, input)
	if errors.Is(err, pluginv2.ErrHostActorDelegationUnavailable) {
		return claimedIcon{}, errIconUploadUnsupported
	}
	if err != nil {
		return claimedIcon{}, iconError("icon_upload_failed",
			"could not prepare the claim request", http.StatusBadGateway)
	}
	request.ExpectedRevision = strings.TrimSpace(revision)
	result, err := host.Commands.Execute(ctx, request)
	if err != nil {
		return claimedIcon{}, iconError("icon_upload_failed",
			"could not reach the host command service", http.StatusBadGateway)
	}
	if reason := commandFailureReason(result); reason != "" {
		return claimedIcon{}, attachmentFailure(reason)
	}
	output := pluginv2.TypedDocumentValues(result.GetOutput())
	claimed := claimedIcon{
		PublicID: stringValue(output["publicId"]),
		URL:      stringValue(output["url"]),
		Revision: stringValue(output["revision"]),
	}
	if claimed.PublicID == "" {
		// 输出缺失时退回入参：claim 已提交，publicId 就是刚才那个。
		claimed.PublicID = publicID
	}
	if claimed.URL == "" {
		claimed.URL = attachmentMediaURL(claimed.PublicID)
	}
	return claimed, nil
}

// releaseIcon 删掉该应用的附件引用，让孤儿回收重新生效（幂等：没有引用时
// released=0，不报错）。只删引用，不删对象——回收期本身就是误删宽限期。
func (s *oauthServer) releaseIcon(
	ctx context.Context,
	parent *protocolwire.RequestContext,
	clientID string,
) error {
	host, err := s.hostClient()
	if err != nil || host == nil {
		return errIconUploadUnsupported
	}
	input, err := pluginv2.NewTypedDocument(attachmentReleaseInputSchema, map[string]any{
		"context": attachmentContext(clientID),
	})
	if err != nil {
		return iconError("icon_upload_failed", "could not encode the release request", http.StatusInternalServerError)
	}
	request, err := host.DelegatedCommandRequest(parent, attachmentReleaseCommandID, attachmentCommandVersion, input)
	if errors.Is(err, pluginv2.ErrHostActorDelegationUnavailable) {
		return errIconUploadUnsupported
	}
	if err != nil {
		return iconError("icon_upload_failed", "could not prepare the release request", http.StatusBadGateway)
	}
	result, err := host.Commands.Execute(ctx, request)
	if err != nil {
		return iconError("icon_upload_failed", "could not reach the host command service", http.StatusBadGateway)
	}
	if reason := commandFailureReason(result); reason != "" {
		return attachmentFailure(reason)
	}
	return nil
}

// releaseIconQuietly 是删除 / 回滚路径用的 best-effort release：失败只记日志，
// 不阻塞用户动作。插件卸载时 Host 会兜底释放本扩展的全部引用；release 失败最多
// 让附件多留一个回收期（默认 30 天），比「删不掉应用」划算得多。
func (s *oauthServer) releaseIconQuietly(
	ctx context.Context,
	parent *protocolwire.RequestContext,
	clientID string,
	cause string,
) {
	if err := s.releaseIcon(ctx, parent, clientID); err != nil {
		log.Printf("oauth2-provider: best-effort attachment release failed (%s, %s): %v",
			cause, clientID, err)
	}
}

// commandFailureReason 从命令结果里取出 Host 的稳定失败码；成功返回空串。
func commandFailureReason(result *hostwire.CommandResult) string {
	if result == nil {
		return "host.command_unavailable"
	}
	if detail := result.GetError(); detail != nil && strings.TrimSpace(detail.GetReason()) != "" {
		return strings.TrimSpace(detail.GetReason())
	}
	switch result.GetState() {
	case hostwire.CommandState_COMMAND_STATE_COMMITTED, hostwire.CommandState_COMMAND_STATE_REPLAYED:
		return ""
	case hostwire.CommandState_COMMAND_STATE_REJECTED:
		return "host.command_rejected"
	default:
		return "host.command_unavailable"
	}
}

// attachmentFailure 把 Host 的稳定错误码翻译成插件稳定码 + HTTP 状态。
// 映射依据 docs/extensions/host-api-v2.md「Extension attachment commands」。
func attachmentFailure(reason string) oauthError {
	switch reason {
	case "host.attachment_revision_conflict":
		return iconError("icon_revision_conflict",
			"the uploaded image changed before it could be claimed; upload it again", http.StatusConflict)
	case "host.attachment_extension_too_large":
		return iconError("icon_too_large",
			"the image exceeds the host limit (png, jpeg or webp, at most 512 KiB)", http.StatusRequestEntityTooLarge)
	case "host.attachment_extension_forbidden":
		return iconError("icon_forbidden",
			"the image is not owned by you or is already used by another feature", http.StatusForbidden)
	case "host.attachment_extension_reference_limit":
		return iconError("icon_reference_limit",
			"this extension reached its image reference limit", http.StatusConflict)
	case "host.attachment_not_found":
		return iconError("icon_attachment_missing",
			"the uploaded image no longer exists; upload it again", http.StatusNotFound)
	case "host.attachment_extension_invalid", "host.attachment_extension_conflict":
		return iconError("icon_invalid",
			"the host rejected the image (png, jpeg or webp only, at most 512 KiB)", http.StatusBadRequest)
	case "host.command_authority_denied":
		return iconError("icon_upload_unsupported",
			"this exact package is not granted host commands; re-approve the plugin", http.StatusConflict)
	case "host.command_actor_delegation_required", "host.command_actor_permission_denied":
		return iconError("icon_upload_unsupported",
			"icon upload is unavailable for this account or host version; use an image URL", http.StatusConflict)
	default:
		return iconError("icon_upload_failed",
			"the host could not claim the image", http.StatusBadGateway)
	}
}

// attachmentMediaURL 是 Core 的稳定媒体地址（与宿主域名无关）。
func attachmentMediaURL(publicID string) string {
	trimmed := strings.TrimSpace(publicID)
	if trimmed == "" {
		return ""
	}
	return "/media/attachments/" + trimmed
}

// applyIconChoice 落实「上传与 URL 互斥」的写路径语义（任务书 §4.2 / §8）：
//
//	只提交 attachmentPublicId → claim → 存 publicId、清 LogoURL
//	只提交非空 logoUrl      → 先 release（若原来有附件）→ 存 URL、清 publicId
//	两个都提交为空          → 先 release（若原来有附件）→ 清两个字段，回到生成图标
//	两个键都没提交          → 没动图标，原样保留
//
// 漏掉 release 就会留下永不回收的孤儿附件，所以「切走」与「清除」两条分支都必须
// 先 release 再落库；release 失败直接让整次保存失败，不给用户「看起来成功了、
// 其实图还挂着」的假象。
func (s *oauthServer) applyIconChoice(
	ctx context.Context,
	request *pluginwire.RouteRequest,
	record *clientRecord,
	existing clientRecord,
	values map[string]any,
) error {
	_, hasAttachment := values["attachmentPublicId"]
	_, hasLogoURL := values["logoUrl"]
	if !hasAttachment && !hasLogoURL {
		return nil
	}
	publicID := strings.TrimSpace(bodyString(values, "attachmentPublicId"))
	logoURL := strings.TrimSpace(bodyString(values, "logoUrl"))
	revision := strings.TrimSpace(bodyString(values, "attachmentRevision"))
	hadAttachment := strings.TrimSpace(existing.AttachmentPublicID) != ""
	parent := request.GetContext()

	if publicID != "" && logoURL != "" {
		return invalidRequest("attachmentPublicId and logoUrl are mutually exclusive; submit only one")
	}

	if publicID != "" {
		// clientId 决定 claim 的 context，所以调用前必须已经定下来（见 buildClient）。
		if err := requireIdempotencyKey(parent); err != nil {
			return err
		}
		claimed, err := s.claimIcon(ctx, parent, record.ClientID, publicID, revision)
		if err != nil {
			return err
		}
		record.AttachmentPublicID = claimed.PublicID
		record.LogoURL = ""
		return nil
	}

	if logoURL != "" {
		if !validLogoURL(logoURL) {
			return invalidRequest("logo URL must be https (http on loopback is allowed), without a fragment, at most 2048 characters")
		}
		if hadAttachment {
			if err := requireIdempotencyKey(parent); err != nil {
				return err
			}
			if err := s.releaseIcon(ctx, parent, record.ClientID); err != nil {
				return err
			}
		}
		record.LogoURL = logoURL
		record.AttachmentPublicID = ""
		return nil
	}

	// 显式提交空值 = 清除图标。
	if hadAttachment {
		if err := requireIdempotencyKey(parent); err != nil {
			return err
		}
		if err := s.releaseIcon(ctx, parent, record.ClientID); err != nil {
			return err
		}
	}
	record.LogoURL = ""
	record.AttachmentPublicID = ""
	return nil
}

// requireIdempotencyKey 保证「要调 Host Command 的保存」一定带了幂等键。
//
// Host 只在 HTTP 请求带 Idempotency-Key 时才签发 actor 委派；缺了它
// DelegatedCommandRequest 会返回 ErrHostActorDelegationUnavailable，与「旧宿主」
// 无法区分。所以在调命令之前先把这种情况拦成明确的 400，而不是误报成
// icon_upload_unsupported（那会让前端去展开 URL 输入，掩盖真正的 bug）。
func requireIdempotencyKey(parent *protocolwire.RequestContext) error {
	if parent == nil || strings.TrimSpace(parent.GetIdempotencyKey()) == "" {
		return iconError("idempotency_key_required",
			"this request must carry an Idempotency-Key header", http.StatusBadRequest)
	}
	return nil
}

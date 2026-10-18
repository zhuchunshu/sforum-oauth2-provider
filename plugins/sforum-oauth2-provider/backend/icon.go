package main

import (
	"strings"
)

// 应用 icon 统一规范（单一事实源）。
//
// 三处前端——授权确认页、前台「我的应用 / 已授权」卡片、管理端应用列表——
// 以前各画各的头像（甚至管理端没有头像），算法必然漂移。这里把 icon 的渲染
// 描述符交给后端计算并随 API 下发，前端只按描述符渲染，保证逐像素一致。
//
// 三种来源（互斥，优先级从上到下）：
//   - attachment：应用上传了图片，Core 把它登记成本扩展的公开资产，
//     url 是宿主相对地址 /media/attachments/<publicId>；
//   - url       ：应用填了图片地址（历史数据 + 旧宿主降级 + 想用自己的图床）；
//   - generated ：两者都没有，按种子生成 **GitHub 风格的 identicon**：
//     方型、浅灰纯色底（#f0f0f0）、5×5 镜像像素块（单色、哈希决定色相），
//     布局 / 填充规则 / 色块观感全部对齐 GitHub 的默认头像（见 identicon.go）。
//
// attachment 与 url 都下发 url，前端只在 url 存在时叠 <img>（加载失败回退
// 生成图标），因此渲染分支只有一条，来源差异全部收在描述符里。
//
// 生成算法是确定性的：同一种子永远得到同一图案与同一颜色，不同应用自然错开。

// iconBackground 是生成图标的纯色底（GitHub identicon 同款浅灰，不随主题变化）。
const iconBackground = "#f0f0f0"

// 图标来源 kind。schema 枚举（clients / authorizations / consent-summary /
// client-write 响应）必须与这三个值一致。
const (
	iconKindAttachment = "attachment"
	iconKindURL        = "url"
	iconKindGenerated  = "generated"
)

// iconSource 是描述符的唯一输入：谁提供这张图、图在哪。
type iconSource struct {
	Kind     string
	URL      string
	PublicID string
}

// clientIconSource 按 attachment > url > generated 的优先级解析一条应用记录。
func clientIconSource(record clientRecord) iconSource {
	if publicID := strings.TrimSpace(record.AttachmentPublicID); publicID != "" {
		return iconSource{Kind: iconKindAttachment, URL: attachmentMediaURL(publicID), PublicID: publicID}
	}
	if logoURL := strings.TrimSpace(record.LogoURL); logoURL != "" {
		return iconSource{Kind: iconKindURL, URL: logoURL}
	}
	return iconSource{Kind: iconKindGenerated}
}

// appIconDescriptor 构造统一的 icon 描述符。seed 决定生成形态的图案与色块颜色，
// 优先用稳定且公开的标识（clientId，缺失回退应用名）；source 决定这张图从哪来
// （见 iconSource）。generated 形态永远带全套兜底字段，attachment/url 形态额外带 url。
func appIconDescriptor(seed, name string, source iconSource) map[string]any {
	key := firstNonEmpty(strings.TrimSpace(seed), strings.TrimSpace(name))
	descriptor := map[string]any{
		"kind":       iconKindGenerated,
		"pattern":    identiconPattern(key),
		"background": iconBackground,
		"color":      identiconColor(key),
	}
	kind := source.Kind
	if kind != iconKindAttachment && kind != iconKindURL {
		kind = iconKindGenerated
	}
	url := strings.TrimSpace(source.URL)
	if kind == iconKindGenerated || url == "" {
		return descriptor
	}
	descriptor["kind"] = kind
	descriptor["url"] = url
	return descriptor
}

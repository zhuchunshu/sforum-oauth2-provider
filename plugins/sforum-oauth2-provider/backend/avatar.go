package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// 用户头像取值（userinfo 的 `avatar` 字段）。
//
// 宿主把头像投影成 URL 字符串（core-safe 的 `avatar`），但只有 uploaded /
// gravatar / static 三种来源才有 URL；站点默认的 initials（首字头像）由前端
// SFAvatar 现场渲染，宿主侧 URL 为空。直接按「非空才下发」处理会让默认安装
// 下的 avatar scope 永远拿不到任何值（已实测：本机 81 个用户档案 0 个上传头像）。
//
// 因此这里分两步：
//  1. 宿主给了 URL 就使用（上传头像为站点相对路径 /media/avatars/<publicId>，
//     会按站点公开地址补成绝对链接；gravatar / 静态默认图本来就是绝对地址）；
//  2. 没有 URL 时生成确定性的 SVG data URL：**GitHub 风格的 identicon**——浅灰
//     方底（#f0f0f0）+ 5×5 镜像像素块（哈希决定色相、纯色，无文字），与默认应用
//     图标同一套算法（backend/identicon.go）。同一用户永远得到同一张图，客户端
//     可安全缓存。
//
// 选 data URL 而不是新增图片路由：不引入公开端点、不需要额外契约与缓存策略，
// 且插件拿不到站点绝对地址（Host 不给 site.url，Host 头在插件路由里被过滤），
// 相对地址对第三方服务端同样不可用。SVG 内容完全由服务端生成，没有脚本、
// 文字或外部引用。

const (
	avatarFallbackSeed  = "user"
	avatarDataURLPrefix = "data:image/svg+xml;base64,"
)

// avatarValue 返回 userinfo 的 avatar 值：宿主 URL 优先并补成绝对地址，
// 缺失时生成兜底头像。baseURL 为空时相对地址原样返回（尽力而为）。
func avatarValue(profile userProfile, baseURL string) string {
	if raw := strings.TrimSpace(profile.AvatarURL); raw != "" {
		return absoluteURL(baseURL, raw)
	}
	return generatedAvatarDataURL(avatarSeed(profile))
}

// avatarValueFor 是 userinfo 的入口：只有宿主给出相对地址时才去解析站点公开地址，
// 绝对地址（Gravatar / 静态图）与首字头像都不需要额外网络请求。
func (s *oauthServer) avatarValueFor(ctx context.Context, profile userProfile) string {
	raw := strings.TrimSpace(profile.AvatarURL)
	if raw == "" {
		return generatedAvatarDataURL(avatarSeed(profile))
	}
	if isAbsoluteURL(raw) {
		return raw
	}
	return absoluteURL(s.publicBaseURL(ctx), raw)
}

// avatarSeed 是 identicon 图案与颜色的种子。站点 SFAvatar 用显示名
// （avatar.alt 回退 name），因此这里也优先 displayName。
func avatarSeed(profile userProfile) string {
	return firstNonEmpty(
		strings.TrimSpace(profile.DisplayName),
		strings.TrimSpace(profile.Username),
		avatarFallbackSeed,
	)
}

// generatedAvatarDataURL 生成 128×128 的 GitHub 风格 identicon（SVG → base64 data URL）：
// 6×6 viewBox 里 5×5 网格居中（0.5 单位边距 = GitHub identicon 的 8.3%），
// 浅灰方底 + 纯色像素块，无文字。
func generatedAvatarDataURL(seed string) string {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		seed = avatarFallbackSeed
	}
	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="128" height="128" viewBox="0 0 6 6">`+
			`<rect width="6" height="6" fill="%s"/>`+
			`<path d="%s" transform="translate(0.5 0.5)" fill="%s" shape-rendering="crispEdges"/>`+
			`</svg>`,
		iconBackground,
		identiconPath(identiconPattern(seed)),
		identiconColor(seed),
	)
	return avatarDataURLPrefix + base64.StdEncoding.EncodeToString([]byte(svg))
}

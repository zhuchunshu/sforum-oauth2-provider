package main

// identicon 生成 GitHub 风格的应用默认图标（浅灰方底 + 5×5 镜像像素块）。
// 该实现是 backend/icon.go 与 userinfo 兜底头像的单一事实源。
import (
	"fmt"
	"strings"
	"unicode/utf16"
)

// 默认图标的 identicon 生成器（Go 侧单一事实源）。
//
// 形态参考 GitHub：5×5 网格、左右镜像、偶校验填充。差异点只有哈希原语——
// 这里复用站点 SFAvatar 的 `hash = hash*31 + codeUnit`（UTF-16 码元），
// 而不是 GitHub 的 MD5：同一个原语同时决定色板索引，Go 与三份 JS 镜像
// （前台 apps.mjs、管理端 clients.mjs、确认页内联脚本）可以逐位复现，
// 不需要在浏览器里实现 MD5。
//
// 不追求与 GitHub 的位级一致：种子是 client_id / 显示名，本来就和 GitHub
// 的用户 id 无关；对齐的是布局（5×5 镜像）与填充规则（nibble 偶校验）。

const (
	// identiconFallbackSeed 是空种子的兜底，保证永远有一张可渲染的图案。
	identiconFallbackSeed = "sforum"
	// identiconFallbackCell 是全空图案时强制点亮的格子（中心），避免「隐形图标」。
	identiconFallbackCell = 12
	// identiconSaturation / identiconLightness 取自 GitHub identicon 的实测区间
	// （抽样 4 个默认头像：S 50–65%、L 56–70%），保证色块观感与 GitHub 一致。
	identiconSaturation = 60
	identiconLightness  = 63
)

// stableHash 复刻站点 SFAvatar 的 hashStableTone：按 UTF-16 码元累加，
// 因此 Go 侧必须先转 UTF-16，否则星平面字符会与站点取到不同结果。
func stableHash(seed string) uint32 {
	units := utf16.Encode([]rune(seed))
	hash := uint32(0)
	for _, unit := range units {
		hash = hash*31 + uint32(unit)
	}
	return hash
}

// identiconPattern 返回 25 个 '0'/'1'（行优先、上→下、左→右）。
//
// 只有 15 个独立格子：5 行 × 左中三列，按 GitHub 的顺序用两个 32 位哈希的
// nibble 偶校验决定填充；第 4、5 列镜像第 2、1 列。
func identiconPattern(seed string) string {
	base := strings.TrimSpace(seed)
	if base == "" {
		base = identiconFallbackSeed
	}
	primary := stableHash(base)
	secondary := stableHash(base + "\u001f")

	var filled [15]bool
	for index := 0; index < 15; index++ {
		var nibble uint32
		if index < 8 {
			nibble = (primary >> (4 * uint(index))) & 0xF
		} else {
			nibble = (secondary >> (4 * uint(index-8))) & 0xF
		}
		filled[index] = nibble%2 == 0
	}

	cells := [25]byte{}
	painted := false
	for row := 0; row < 5; row++ {
		for column := 0; column < 3; column++ {
			if filled[row*3+column] {
				cells[row*5+column] = '1'
				painted = true
			} else {
				cells[row*5+column] = '0'
			}
		}
		cells[row*5+3] = cells[row*5+1]
		cells[row*5+4] = cells[row*5+0]
	}
	if !painted {
		cells[identiconFallbackCell] = '1'
	}
	return string(cells[:])
}

// identiconColor 返回 identicon 色块的纯色（CSS 颜色串）。GitHub 的 identicon
// 用同一份哈希同时决定图案与颜色；这里也用 stableHash，色相取模 360。
func identiconColor(seed string) string {
	base := strings.TrimSpace(seed)
	if base == "" {
		base = identiconFallbackSeed
	}
	return fmt.Sprintf("hsl(%d %d%% %d%%)", stableHash(base)%360, identiconSaturation, identiconLightness)
}

// identiconPath 把 25 格图案转成 SVG 路径（单位 = 格子，viewBox 0 0 5 5）。
// 每个亮格是 1×1 的方点；渲染端用 shape-rendering="crispEdges" 避免接缝。
func identiconPath(pattern string) string {
	var builder strings.Builder
	for index := 0; index < len(pattern) && index < 25; index++ {
		if pattern[index] != '1' {
			continue
		}
		fmt.Fprintf(&builder, "M%d %dh1v1h-1z", index%5, index/5)
	}
	return builder.String()
}

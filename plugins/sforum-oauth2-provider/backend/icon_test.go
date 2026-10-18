package main

import (
	"strings"
	"testing"
)

// icon 规范是四处渲染（授权确认页 / 前台卡片 / 管理端列表 / userinfo 头像）的
// 单一事实源，任何漂移都会让默认图标在不同界面长得不一样。这里固化生成算法的
// 关键不变量：25 格、左右镜像、确定性、偶校验填充、SFAvatar 色板。

func TestIdenticonPatternShape(t *testing.T) {
	for _, seed := range []string{"sfo_client_abc", "My Blog", "知乎", "", "   "} {
		pattern := identiconPattern(seed)
		if len(pattern) != 25 {
			t.Fatalf("identiconPattern(%q) length = %d, want 25", seed, len(pattern))
		}
		for _, cell := range pattern {
			if cell != '0' && cell != '1' {
				t.Fatalf("identiconPattern(%q) contains %q, want only 0/1", seed, cell)
			}
		}
		if !strings.Contains(pattern, "1") {
			t.Fatalf("identiconPattern(%q) is empty; the center cell must be forced on", seed)
		}
	}
}

func TestIdenticonPatternMirrored(t *testing.T) {
	for _, seed := range []string{"sfo_client_abc", "My Blog", "知乎"} {
		pattern := identiconPattern(seed)
		for row := 0; row < 5; row++ {
			cells := pattern[row*5 : row*5+5]
			if cells[0] != cells[4] || cells[1] != cells[3] {
				t.Fatalf("identiconPattern(%q) row %d is not mirrored: %q", seed, row, cells)
			}
		}
	}
}

func TestIdenticonPatternStable(t *testing.T) {
	if identiconPattern("sfo_client_same") != identiconPattern("sfo_client_same") {
		t.Fatal("same seed must produce the same pattern")
	}
	if identiconPattern("sfo_client_a") == identiconPattern("sfo_client_b") {
		t.Fatal("different seeds should (in practice) produce different patterns")
	}
	// 空种子回退到固定种子，保证永远有图案。
	if identiconPattern("") != identiconPattern("sforum") {
		t.Fatal("empty seed must fall back to the identicon fallback seed")
	}
}

// golden 值固定算法：哈希原语、nibble 顺序、镜像方式任何一处改动都会让它变红。
func TestIdenticonPatternGolden(t *testing.T) {
	cases := map[string]string{
		"sfo_client_abc": "0000000000001001111111011",
		"My Blog":        "1000111011000001010110001",
		"知乎":             "0101001110111110111001010",
	}
	for seed, want := range cases {
		if got := identiconPattern(seed); got != want {
			t.Fatalf("identiconPattern(%q) = %q, want %q", seed, got, want)
		}
	}
}

func TestIdenticonPath(t *testing.T) {
	pattern := identiconPattern("sfo_client_abc")
	path := identiconPath(pattern)
	if filled := strings.Count(pattern, "1"); strings.Count(path, "h1v1h-1z") != filled {
		t.Fatalf("path cells = %d, want %d (%q)", strings.Count(path, "h1v1h-1z"), filled, path)
	}
	if !strings.HasPrefix(path, "M") {
		t.Fatalf("path must start with a move command: %q", path)
	}
}

func TestStableHashMatchesSiteAlgorithm(t *testing.T) {
	// 站点 SFAvatar 的 hashStableTone 用 UTF-16 码元累加（charCodeAt）。
	if stableHash("Alice") != stableHash("Alice") {
		t.Fatal("stableHash must be deterministic")
	}
	if stableHash("") != 0 {
		t.Fatalf("stableHash(\"\") = %d, want 0", stableHash(""))
	}
	// 星平面字符必须按 UTF-16 代理对累加，不能按 rune。
	if stableHash("𝄞") == stableHash("𝄞x") {
		t.Fatal("stableHash must consume every UTF-16 code unit")
	}
}

func TestIdenticonColor(t *testing.T) {
	for _, seed := range []string{"sfo_client_abc", "Alice", "", "知乎"} {
		color := identiconColor(seed)
		if identiconColor(seed) != color {
			t.Fatalf("identiconColor(%q) is not stable", seed)
		}
		if !strings.HasPrefix(color, "hsl(") || !strings.HasSuffix(color, "%)") {
			t.Fatalf("identiconColor(%q) = %q, want an hsl() color", seed, color)
		}
		// S/L 取自 GitHub identicon 实测区间（S 50–65%、L 56–70%）。
		if !strings.Contains(color, " 60% 63%)") {
			t.Fatalf("identiconColor(%q) = %q, want the GitHub-like saturation/lightness", seed, color)
		}
	}
	if identiconColor("sfo_client_a") == identiconColor("sfo_client_b") {
		t.Log("two seeds share a hue; acceptable but worth knowing")
	}
}

func TestAppIconDescriptorGenerated(t *testing.T) {
	descriptor := appIconDescriptor("sfo_client_abc", "My Blog", iconSource{Kind: iconKindGenerated})
	if descriptor["kind"] != iconKindGenerated {
		t.Fatalf("kind = %v, want generated", descriptor["kind"])
	}
	if descriptor["pattern"] != identiconPattern("sfo_client_abc") {
		t.Fatalf("pattern = %v, want the seed-derived pattern", descriptor["pattern"])
	}
	if descriptor["background"] != iconBackground {
		t.Fatalf("background = %v, want %s", descriptor["background"], iconBackground)
	}
	if iconBackground != "#f0f0f0" {
		t.Fatalf("iconBackground = %q, want the GitHub identicon gray", iconBackground)
	}
	color, _ := descriptor["color"].(string)
	if color != identiconColor("sfo_client_abc") {
		t.Fatalf("color = %q, want the seed-derived identicon color", color)
	}
	if !strings.HasPrefix(color, "hsl(") {
		t.Fatalf("color = %q, want an hsl() string", color)
	}
	if _, hasURL := descriptor["url"]; hasURL {
		t.Fatal("generated icon must not carry a url")
	}
}

func TestAppIconDescriptorSeedFallsBackToName(t *testing.T) {
	descriptor := appIconDescriptor("", "My Blog", iconSource{Kind: iconKindGenerated})
	if descriptor["pattern"] != identiconPattern("My Blog") {
		t.Fatalf("pattern = %v, want the name-derived pattern", descriptor["pattern"])
	}
}

func TestAppIconDescriptorURLSource(t *testing.T) {
	logo := "https://cdn.example.com/logo.png"
	descriptor := appIconDescriptor("sfo_client_abc", "My Blog", iconSource{Kind: iconKindURL, URL: logo})
	if descriptor["kind"] != iconKindURL {
		t.Fatalf("kind = %v, want %s", descriptor["kind"], iconKindURL)
	}
	if descriptor["url"] != logo {
		t.Fatalf("url = %v, want %s", descriptor["url"], logo)
	}
	// 带 url 的形态仍要带生成兜底字段，供 <img> 加载失败时回退。
	if descriptor["pattern"] == "" || descriptor["color"] == "" || descriptor["background"] == "" {
		t.Fatal("url icon must still carry generated fallback fields")
	}
}

func TestAppIconDescriptorAttachmentSource(t *testing.T) {
	descriptor := appIconDescriptor("sfo_client_abc", "My Blog", iconSource{
		Kind: iconKindAttachment, URL: attachmentMediaURL("abc123def456"), PublicID: "abc123def456",
	})
	if descriptor["kind"] != iconKindAttachment {
		t.Fatalf("kind = %v, want %s", descriptor["kind"], iconKindAttachment)
	}
	if descriptor["url"] != "/media/attachments/abc123def456" {
		t.Fatalf("url = %v", descriptor["url"])
	}
}

// 优先级：attachment > url > generated。历史数据里两个字段可能同时存在
// （切到 URL 时 release 失败被容忍过），描述符必须永远报 attachment。
func TestClientIconSourcePriority(t *testing.T) {
	both := clientRecord{AttachmentPublicID: "abc123def456", LogoURL: "https://cdn.example.com/logo.png"}
	if source := clientIconSource(both); source.Kind != iconKindAttachment || source.PublicID != "abc123def456" {
		t.Fatalf("attachment must win, got %#v", source)
	}
	onlyURL := clientRecord{LogoURL: "https://cdn.example.com/logo.png"}
	if source := clientIconSource(onlyURL); source.Kind != iconKindURL || source.URL != onlyURL.LogoURL {
		t.Fatalf("url fallback, got %#v", source)
	}
	if source := clientIconSource(clientRecord{}); source.Kind != iconKindGenerated {
		t.Fatalf("empty record must be generated, got %#v", source)
	}
}

func TestAppIconDescriptorStableColor(t *testing.T) {
	a := appIconDescriptor("sfo_client_same", "Alpha", iconSource{Kind: iconKindGenerated})
	b := appIconDescriptor("sfo_client_same", "Alpha", iconSource{Kind: iconKindGenerated})
	if a["pattern"] != b["pattern"] || a["color"] != b["color"] {
		t.Fatal("same seed must produce the same pattern and tone")
	}
}

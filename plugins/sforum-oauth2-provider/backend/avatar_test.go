package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

// 头像兜底逻辑的单测。兜底头像是 GitHub 风格的 identicon（浅灰方底 + 5×5 镜像
// 像素块、单色、无文字），与默认应用图标共用 backend/identicon.go 的算法。

func TestGeneratedAvatarDataURL(t *testing.T) {
	first := generatedAvatarDataURL("SFO Smoke Admin")
	if !strings.HasPrefix(first, avatarDataURLPrefix) {
		t.Fatalf("data URL prefix missing: %.60q", first)
	}
	svg, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(first, avatarDataURLPrefix))
	if err != nil {
		t.Fatalf("decode data URL: %v", err)
	}
	document := string(svg)
	for _, fragment := range []string{
		`<svg xmlns="http://www.w3.org/2000/svg"`,
		`viewBox="0 0 6 6"`,
		`fill="#f0f0f0"`,
		`shape-rendering="crispEdges"`,
		identiconColor("SFO Smoke Admin"),
	} {
		if !strings.Contains(document, fragment) {
			t.Fatalf("svg missing %q: %s", fragment, document)
		}
	}
	// GitHub identicon 是纯像素图：没有文字，也不是圆形底。
	if strings.Contains(document, "<text") {
		t.Fatalf("generated avatar must not render initials: %s", document)
	}
	if strings.Contains(document, "<circle") {
		t.Fatalf("generated avatar must be a square identicon canvas: %s", document)
	}
	// 像素块图案必须落在 5×5 网格里（与默认应用图标同一算法）。
	if path := identiconPath(identiconPattern("SFO Smoke Admin")); !strings.Contains(document, path) {
		t.Fatalf("svg missing the identicon path %q: %s", path, document)
	}
	// 确定性：同一用户每次得到同一张图（客户端可安全缓存）。
	if second := generatedAvatarDataURL("SFO Smoke Admin"); second != first {
		t.Fatal("generated avatar must be deterministic")
	}
	// 不同种子必须得到不同图案（至少这一对）。
	if generatedAvatarDataURL("Alice") == generatedAvatarDataURL("Bob") {
		t.Fatal("different seeds must produce different identicons")
	}
	// 空种子回退到固定种子。
	if generatedAvatarDataURL("") != generatedAvatarDataURL(avatarFallbackSeed) {
		t.Fatal("empty seed must fall back to the fixed avatar seed")
	}
}

func TestGeneratedAvatarUsesCJKSeed(t *testing.T) {
	svg := string(mustDecodeDataURL(t, generatedAvatarDataURL("种子用户2")))
	if !strings.Contains(svg, identiconColor("种子用户2")) {
		t.Fatalf("CJK seed must drive the identicon color: %s", svg)
	}
	if strings.Contains(svg, "<text") {
		t.Fatalf("generated avatar must stay text-free: %s", svg)
	}
}

func TestAvatarValuePrefersHostURL(t *testing.T) {
	const base = "http://127.0.0.1:3000"
	profile := userProfile{DisplayName: "Alice", AvatarURL: " /media/avatars/abc "}
	if got := avatarValue(profile, base); got != "http://127.0.0.1:3000/media/avatars/abc" {
		t.Fatalf("avatarValue = %q, want the absolute host URL", got)
	}
	// 已经是绝对地址时原样返回，不重复拼接。
	absolute := userProfile{DisplayName: "Alice", AvatarURL: "https://cdn.example.com/a.png"}
	if got := avatarValue(absolute, base); got != "https://cdn.example.com/a.png" {
		t.Fatalf("avatarValue = %q, want the absolute host URL untouched", got)
	}
	profile.AvatarURL = ""
	got := avatarValue(profile, base)
	if !strings.HasPrefix(got, avatarDataURLPrefix) {
		t.Fatalf("empty host URL must fall back to a generated avatar, got %.60q", got)
	}
	// 种子优先 displayName；缺失时回退 username（两者得到不同 identicon）。
	fallback := avatarValue(userProfile{Username: "sfo_smoke_admin"}, base)
	if fallback == got {
		t.Fatal("displayName and username must not collapse to the same identicon")
	}
	if !strings.HasPrefix(fallback, avatarDataURLPrefix) {
		t.Fatalf("username fallback must also be a generated avatar, got %.60q", fallback)
	}
}

func TestAvatarValueWithoutBaseKeepsRelativeURL(t *testing.T) {
	// 拿不到站点地址时保留相对路径，绝不猜测域名。
	got := avatarValue(userProfile{AvatarURL: "/media/avatars/abc"}, "")
	if got != "/media/avatars/abc" {
		t.Fatalf("avatarValue = %q, want the relative URL unchanged", got)
	}
}

func mustDecodeDataURL(t *testing.T, value string) []byte {
	t.Helper()
	if !strings.HasPrefix(value, avatarDataURLPrefix) {
		t.Fatalf("not a generated avatar data URL: %.60q", value)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, avatarDataURLPrefix))
	if err != nil {
		t.Fatalf("decode data URL: %v", err)
	}
	return decoded
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

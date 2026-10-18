package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 插件公开页的模板与前端脚本是一份精确制品：模板给出挂载点，apps.mjs 往里面填数据。
// 两者只在内容摘要上绑定，靠人眼保证选择器一致。2026-10-16 实测踩到过：模板换了类名、
// 脚本还在找旧类名，页面只剩骨架、控制台无报错——因此这里做双向文本比对。
//
// 这里不执行 JS，只验证「脚本声明的挂载点都存在于模板」+「模板不再复刻宿主左栏」，
// 两者都是纯文本事实，改动任一文件而漏改另一处会立刻变红。

func packageFile(t *testing.T, parts ...string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(append([]string{".."}, parts...)...))
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", strings.Join(parts, "/"), err)
	}
	return string(body)
}

func pageTemplate(t *testing.T) string { return packageFile(t, "templates", "oauth-apps.html") }
func pageScript(t *testing.T) string   { return packageFile(t, "frontend", "public", "apps.mjs") }

// appShellIsland 是左栏岛的唯一合法形态：只允许 active 一个属性，且必须非空。
var appShellIsland = regexp.MustCompile(`<sf-account-settings-sidebar\s+active="([^"]+)"\s*></sf-account-settings-sidebar>`)

// mountPoints 从 MOUNT_POINTS 里抽出选择器字面量。
var mountPointEntry = regexp.MustCompile(`(?m)^\s+[A-Za-z][A-Za-z0-9]*:\s*'([^']+)',\s*$`)

// 模板里的 class / id 取值表：挂载点选择器必须在其中出现才算存在。
var (
	classAttr = regexp.MustCompile(`class="([^"]*)"`)
	idAttr    = regexp.MustCompile(`id="([^"]*)"`)
)

func TestPageScriptMountPointsExistInTemplate(t *testing.T) {
	template := pageTemplate(t)
	script := pageScript(t)

	block := regexp.MustCompile(`(?s)const MOUNT_POINTS = \{(.*?)\n\}`).FindStringSubmatch(script)
	if block == nil {
		t.Fatal("apps.mjs: MOUNT_POINTS block not found")
	}
	entries := mountPointEntry.FindAllStringSubmatch(block[1], -1)
	if len(entries) < 10 {
		t.Fatalf("apps.mjs: expected the full mount-point table, found %d entries", len(entries))
	}

	classes := map[string]struct{}{}
	for _, match := range classAttr.FindAllStringSubmatch(template, -1) {
		for _, token := range strings.Fields(match[1]) {
			classes[token] = struct{}{}
		}
	}
	ids := map[string]struct{}{}
	for _, match := range idAttr.FindAllStringSubmatch(template, -1) {
		ids[match[1]] = struct{}{}
	}

	for _, entry := range entries {
		selector := entry[1]
		switch {
		case strings.HasPrefix(selector, ".") && strings.Contains(selector, " "):
			// 后代选择器只校验最后一段类名，模板里不保证同样的祖先结构。
			parts := strings.Fields(selector)
			if _, ok := classes[strings.TrimPrefix(parts[len(parts)-1], ".")]; !ok {
				t.Errorf("mount point %q: class %q is absent from templates/oauth-apps.html", selector, parts[len(parts)-1])
			}
		case strings.HasPrefix(selector, "."):
			name := strings.TrimPrefix(selector, ".")
			if _, ok := classes[name]; !ok {
				t.Errorf("mount point %q has no counterpart in templates/oauth-apps.html (no class %q)", selector, name)
			}
		case strings.HasPrefix(selector, "#"):
			name := strings.TrimPrefix(selector, "#")
			if _, ok := ids[name]; !ok {
				t.Errorf("mount point %q has no counterpart in templates/oauth-apps.html (no id %q)", selector, name)
			}
		default:
			t.Errorf("unsupported mount-point selector %q", selector)
		}
	}
}

// 左栏必须来自宿主岛：模板里再出现一份手写左栏标记就是漂移的开始。
func TestPageTemplateDelegatesSidebarToHostIsland(t *testing.T) {
	template := pageTemplate(t)

	match := appShellIsland.FindStringSubmatch(template)
	if match == nil {
		t.Fatal("templates/oauth-apps.html must contain exactly one <sf-account-settings-sidebar active=\"…\"></sf-account-settings-sidebar>")
	}
	if strings.TrimSpace(match[1]) == "" {
		t.Fatal("the sidebar island requires a non-empty active value (host rejects an empty required prop)")
	}
	if count := strings.Count(template, "<sf-account-settings-sidebar"); count != 1 {
		t.Fatalf("expected exactly one sidebar island, found %d", count)
	}

	// 宿主 scoped/自绘左栏的痕迹一律不允许出现：
	// 前者拿不到样式（h1 曾经渲染成 56px），后者的条目会随宿主改版过期。
	for _, forbidden := range []string{
		"sf-public-page-header",
		"sforum-settings__account-nav",
		"sf-home-navigation",
		"sf-responsive-public-sidebar",
	} {
		if strings.Contains(template, forbidden) {
			t.Errorf("templates/oauth-apps.html must not contain %q; the host island owns the sidebar and the page header is plugin-owned", forbidden)
		}
	}

	// 模板净化器会剥掉这些属性，写它们只会得到「看起来设了、实际没有」的假象。
	for _, stripped := range []string{"data-role=", "data-layout=", "aria-selected=", "aria-controls=", "tabindex="} {
		if strings.Contains(template, stripped) {
			t.Errorf("templates/oauth-apps.html uses %q which the host template sanitizer strips; express it with a class and set it at runtime instead", stripped)
		}
	}
}

// 新建/编辑弹窗的表单字段在脚本里只允许出现一份：模板字符串被复制粘贴过两次时，
// 弹窗会渲染出「应用名称 / 应用说明 / 应用图标」各两遍，且重复的 id 让 label 指向
// 第一个输入框（第二个框永远拿不到焦点）。2026-10-18 实测踩到过。
func TestPageScriptModalFieldsAreUnique(t *testing.T) {
	script := pageScript(t)
	block := regexp.MustCompile(`(?s)<form class="sfp-form"[^>]*>(.*?)</form>`).FindStringSubmatch(script)
	if block == nil {
		t.Fatal("apps.mjs: the create/edit modal form block was not found")
	}
	ids := regexp.MustCompile(`\bid="([^"]+)"`).FindAllStringSubmatch(block[1], -1)
	if len(ids) == 0 {
		t.Fatal("apps.mjs: the modal form declares no field ids")
	}
	seen := map[string]int{}
	for _, match := range ids {
		seen[match[1]]++
	}
	for _, id := range []string{"sfp-name", "sfp-description", "sfp-logo", "sfp-type", "sfp-redirect"} {
		if seen[id] != 1 {
			t.Errorf("modal field id=%q appears %d times in apps.mjs, want exactly 1", id, seen[id])
		}
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("modal field id=%q is duplicated (%d occurrences); the form block was copied twice", id, count)
		}
	}
}

// 隐藏状态只能用 .sfp-hidden 类：宿主 CSS 给 .sfp-modal/.sfp-status 设了 display，
// [hidden] 属性会被盖掉（旧实现里模态其实一直是可见的）。
func TestPageScriptHidesThroughClassNotHiddenAttribute(t *testing.T) {
	script := pageScript(t)
	if regexp.MustCompile(`\b\w+\.hidden\s*=`).MatchString(script) {
		t.Error("apps.mjs assigns .hidden= directly; use setHidden() so .sfp-hidden stays the single source of hidden state")
	}
	if !strings.Contains(script, "classList.toggle('sfp-hidden'") {
		t.Error("apps.mjs must toggle .sfp-hidden through setHidden()")
	}
}

// 插件页面没有 iconify 运行时，iconElement() 只读预载缓存：漏在 ensureIcons([...])
// 之外的引用会静默渲染成空按钮。2026-10-18 实测踩到两次——卡片底栏的 ⋯ 菜单按钮
// （lucide:more-horizontal 被 Iconify 改名为 ellipsis，正文只挂在 aliases 指向的
// 规范名下）和复制成功的 ✓（lucide:check 根本没进预载表），两者都是空白。
func TestPageScriptPreloadsEveryIconReference(t *testing.T) {
	script := pageScript(t)

	block := regexp.MustCompile(`(?s)ensureIcons\(\[(.*?)\]\)`).FindStringSubmatch(script)
	if block == nil {
		t.Fatal("apps.mjs: ensureIcons([...]) preload list not found")
	}
	preloaded := map[string]struct{}{}
	for _, match := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(block[1], -1) {
		preloaded[match[1]] = struct{}{}
	}

	for _, match := range regexp.MustCompile(`iconElement\('([^']+)'`).FindAllStringSubmatch(script, -1) {
		if _, ok := preloaded[match[1]]; !ok {
			t.Errorf("iconElement(%q) is used but missing from the ensureIcons([...]) preload list; the button renders empty", match[1])
		}
	}
}

// 页面访问与左栏入口都不能再被 create_clients 锁住：撤销「自己已授权的应用」只需要登录，
// 而页面一旦声明 access=permission，宿主会对普通用户直接 403，导航项也一起消失。
// 2026-10-18 实测：普通用户既看不到入口、硬敲 URL 也进不去，等于没有撤销授权的入口。
func TestPublicPageIsLoginAccessible(t *testing.T) {
	theme := readPackageJSON(t, "theme.json")
	pages, ok := theme["pages"].([]any)
	if !ok || len(pages) == 0 {
		t.Fatalf("theme.json: expected at least one page, got %#v", theme["pages"])
	}
	var page map[string]any
	for _, raw := range pages {
		candidate, _ := raw.(map[string]any)
		if candidate["path"] == "/settings/oauth-apps" {
			page = candidate
			break
		}
	}
	if page == nil {
		t.Fatalf("theme.json: /settings/oauth-apps page is missing")
	}
	if page["access"] != "login" {
		t.Errorf("theme.json: page access = %#v, want \"login\" so every signed-in user can revoke their own authorizations", page["access"])
	}
	if permission, present := page["permission"]; present && permission != "" {
		t.Errorf("theme.json: page declares permission %#v; access=login must not carry one", permission)
	}

	manifest := readPackageJSON(t, "sforum.extension.json")
	navigation, ok := manifest["navigation"].([]any)
	if !ok || len(navigation) == 0 {
		t.Fatal("sforum.extension.json: navigation is empty")
	}
	for _, raw := range navigation {
		item, _ := raw.(map[string]any)
		if item["href"] != "/settings/oauth-apps" {
			continue
		}
		if permission, present := item["permission"]; present && permission != "" {
			t.Errorf("navigation %v: permission = %#v; the entry must stay visible to every signed-in user", item["id"], permission)
		}
		if item["visibility"] != "authenticated" {
			t.Errorf("navigation %v: visibility = %#v, want \"authenticated\"", item["id"], item["visibility"])
		}
		return
	}
	t.Fatal("sforum.extension.json: no navigation entry points at /settings/oauth-apps")
}

func readPackageJSON(t *testing.T, parts ...string) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(packageFile(t, parts...)), &document); err != nil {
		t.Fatalf("parse %s: %v", strings.Join(parts, "/"), err)
	}
	return document
}

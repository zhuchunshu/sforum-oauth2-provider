package main

import (
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pluginwire "github.com/keeptrue/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
)

func TestConsentPageEscapesRequestToken(t *testing.T) {
	page := renderConsentPage(`sfoq_x"><script>alert(1)</script>`, "")
	if strings.Contains(page, `sfoq_x"><script`) {
		t.Fatal("request token must be escaped before being embedded in the page script")
	}
}

// 确认页把 returnURL 内联进 <script> 的字符串字面量，必须走 JSON 编码：
// HTML 转义会把 `&` 变成字面量 `&amp;`（<script> 内容不做实体解码），
// 直接把授权地址拼坏——授权端点拿不到 client_id，只剩一个错误页。
func TestConsentPageEmbedsReturnURLAsJSONLiteral(t *testing.T) {
	returnURL := "/api/oauth/authorize?client_id=sfo_client_x&state=abc&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcb"
	page := renderConsentPage("sfoq_test", returnURL)
	if strings.Contains(page, "&amp;") {
		t.Fatal("return URL must not be HTML-escaped inside the script literal")
	}
	want := "const returnURL = " + jsonLiteral(returnURL)
	if !strings.Contains(page, want) {
		t.Fatalf("return URL must be embedded as a JSON literal, want %s", want)
	}
}

// return 参数来自浏览器地址栏：引号与标签注入都不能逃出字符串字面量。
func TestConsentPageEscapesReturnURL(t *testing.T) {
	page := renderConsentPage("sfoq_test", `/api/oauth/authorize?client_id=x"></script><script>alert(1)</script>`)
	if strings.Contains(page, "</script><script>") {
		t.Fatal("return URL must not be able to close the script block")
	}
	if strings.Contains(page, `client_id=x"`) {
		t.Fatal("quotes inside the return URL must be escaped in the script literal")
	}
}

// 「切换账号」必须是真动作：先退出会话再回登录页，而不是只换一个 href。
func TestConsentPageWiresSwitchAccount(t *testing.T) {
	page := renderConsentPage("sfoq_test", "/api/oauth/authorize?client_id=c&state=s")
	for _, needle := range []string{
		`<a class="switch-account" href="/login"`,
		"function hasAuthorizeReturn",
		"/api/v1/auth/logout",
		"X-Csrf-Token",
		"window.location.assign('/login?redirect=' + encodeURIComponent(returnURL))",
	} {
		if !strings.Contains(page, needle) {
			t.Errorf("consent page must contain %q", needle)
		}
	}
}

// fallbackAuthorizeQuery 只接受指向本站授权端点、且带查询串的相对路径。
func TestFallbackAuthorizeQueryRejectsUnsafeReturn(t *testing.T) {
	server := testServer(t)
	consent := func(returnTo string) string {
		return server.fallbackAuthorizeQuery(&pluginwire.RouteStreamOpen{
			Path: "/api/oauth/consent?request=sfoq_test&return=" + url.QueryEscape(returnTo),
		})
	}

	valid := "/api/oauth/authorize?client_id=c&state=s"
	if got := consent(valid); got != valid {
		t.Fatalf("valid return must be preserved, got %q", got)
	}
	for _, unsafe := range []string{
		"",
		authorizeEndpointPath,
		"//evil.example/api/oauth/authorize?client_id=c",
		"https://evil.example/api/oauth/authorize?client_id=c",
		"/settings",
		"/api/oauth/authorizeevil?client_id=c",
	} {
		if got := consent(unsafe); got != authorizeEndpointPath {
			t.Fatalf("unsafe return %q must fall back to %q, got %q", unsafe, authorizeEndpointPath, got)
		}
	}
}

// 确认页的站点品牌与主题来自宿主公开配置与会话：模板必须带上两条数据通路、
// 新标签页 topbar、色彩模式脚本与头像复刻函数，缺一个都是回归。
func TestConsentPageLoadsSiteChrome(t *testing.T) {
	page := renderConsentPage("sfoq_test", "")
	for _, needle := range []string{
		"/api/v1/web-options",
		"/api/v1/auth/session",
		`id="site-brand"`,
		`target="_blank"`,
		"nuxt-color-mode",
		"data-mode",
		"--sf-accent-soft-light",
		"avatarInitials",
		"AVATAR_TONES",
		"noreferrer",
	} {
		if !strings.Contains(page, needle) {
			t.Errorf("consent page template must contain %q", needle)
		}
	}
}

// 注入前的初始 token 必须是中性灰，不得预设任何站点主题色；
// 品牌与账号一律运行时渲染，模板里不许有硬编码兜底文案。
func TestConsentPageNoHardcodedBrandOrTheme(t *testing.T) {
	page := renderConsentPage("sfoq_test", "")
	if strings.Contains(page, ">IK<") {
		t.Error("consent page must not hardcode the account initials")
	}
	if strings.Contains(page, "SForum 身份服务") {
		t.Error("topbar must show the configured site name, not a hardcoded label")
	}
	if strings.Contains(page, "--sf-accent: #0f766e") {
		t.Error("the page must not start from the pine_teal accent before theme injection")
	}
}

// 拒绝流程回跳后不再渲染任何状态：决策处理必须对非字符串载荷做类型保护。
func TestConsentPageDecisionHandlesNonStringPayload(t *testing.T) {
	page := renderConsentPage("sfoq_test", "")
	if strings.Contains(page, "data.errorDescription || data.error ||") {
		t.Error("decision handling must type-check the payload instead of rendering raw values")
	}
	if !strings.Contains(page, "typeof text === 'string'") {
		t.Error("setStatus must reject non-string input")
	}
}

// 模板内的 module 脚本是 Go 原始字符串，语法错误只能在运行时暴露；
// 抽出来交给 node --check（可用时）提前拦截。三种注入形态都要过：
// 空 return、带查询串的 return、以及试图闭合 <script> 的恶意 return。
func TestConsentPageModuleScriptParses(t *testing.T) {
	cases := []struct {
		name      string
		returnURL string
	}{
		{name: "empty", returnURL: ""},
		{name: "query", returnURL: "/api/oauth/authorize?client_id=c&state=s"},
		{name: "hostile", returnURL: `/api/oauth/authorize?x="></script><script>alert(1)</script>`},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			page := renderConsentPage("sfoq_test", item.returnURL)
			start := strings.Index(page, "<script type=\"module\">")
			end := strings.LastIndex(page, "</script>")
			if start < 0 || end <= start {
				t.Fatal("module script block not found")
			}
			script := page[start+len("<script type=\"module\">") : end]
			if strings.Contains(script, "`") {
				t.Fatal("template JS must stay free of backticks inside the Go raw string")
			}
			dir := t.TempDir()
			path := filepath.Join(dir, "consent.mjs")
			if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
				t.Fatalf("write temp script: %v", err)
			}
			node, err := exec.LookPath("node")
			if err != nil {
				t.Skip("node is not available for syntax checking")
			}
			if out, err := exec.Command(node, "--check", path).CombinedOutput(); err != nil {
				t.Fatalf("module script has a syntax error: %v\n%s", err, out)
			}
		})
	}
}

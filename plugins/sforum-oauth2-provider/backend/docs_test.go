package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginwire "github.com/zhuchunshu/sforum/apps/api/sdk/plugin/v2/gen/sforum/plugin/v2"
)

// 接入说明页是「宿主外壳 + 插件正文」：页面由 theme.json 声明为 view=component，
// 页头/页脚/布局由 Core 渲染，正文由包内 L2 组件挂载，会漂移的三张表由
// GET /api/oauth/docs 提供。这些测试守住三层契约：
//   1. 数据端点与清单/注册表/错误分支一致；
//   2. 页面声明与组件制品一致（不会退化成自绘整页）；
//   3. 组件确实用 Core 的公开页骨架与 .sf-prose，而不是自己造一套外壳。

func docsManifestRoutes(t *testing.T) []string {
	t.Helper()
	manifest := readPackageJSON(t, "sforum.extension.json")
	raw, ok := manifest["routes"].([]any)
	if !ok || len(raw) == 0 {
		t.Fatal("sforum.extension.json: routes is empty")
	}
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		route, _ := item.(map[string]any)
		if path, _ := route["path"].(string); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

// 端点表必须与清单 routes[] 完全同集：既不许漏，也不许编。
func TestDocsEndpointsCoverExactlyTheManifestRoutes(t *testing.T) {
	documented := map[string]struct{}{}
	for _, item := range docsEndpoints {
		documented[item.Path] = struct{}{}
	}
	manifest := map[string]struct{}{}
	for _, path := range docsManifestRoutes(t) {
		manifest[path] = struct{}{}
		if _, ok := documented[path]; !ok {
			t.Errorf("route %s is declared in sforum.extension.json but missing from docsEndpoints", path)
		}
	}
	for path := range documented {
		if _, ok := manifest[path]; !ok {
			t.Errorf("docsEndpoints documents %s which no manifest route serves", path)
		}
	}
}

// 数据端点必须满足自己的 responseSchema（宿主会对 HTTP 路由做同样的校验）。
func TestDocsResponseMatchesSchema(t *testing.T) {
	server := testServer(t)
	response, err := server.handleDocs(testRequest(routeDocsID, map[string]any{}))
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	assertMatchesSchema(t, "docs.response.json", response)
	values := responseValues(t, response)
	if values["ok"] != true {
		t.Fatalf("docs response ok = %#v", values["ok"])
	}
}

// scope 表由 scopeRegistry 生成：新增 scope 只在 constants.go 登记一行，
// 数据端点自动带上，前端无需改。
func TestDocsScopesComeFromTheRegistry(t *testing.T) {
	server := testServer(t)
	response, err := server.handleDocs(testRequest(routeDocsID, map[string]any{}))
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	rows, _ := responseValues(t, response)["scopes"].([]any)
	if len(rows) != len(scopeRegistry) {
		t.Fatalf("docs scopes = %d rows, want %d (one per registered scope)", len(rows), len(scopeRegistry))
	}
	for index, def := range scopeRegistry {
		row, _ := rows[index].(map[string]any)
		if row == nil || row["name"] != def.Name {
			t.Fatalf("docs scope row %d = %#v, want %q", index, rows[index], def.Name)
		}
		description, _ := row["description"].(map[string]any)
		if description == nil || description["zh-CN"] != def.DescZh {
			t.Fatalf("docs scope %q description drifted from scopeRegistry", def.Name)
		}
	}
}

// 错误码表里的每个码都必须真实存在于其它源码文件的错误分支里。
// 扫源码而不是维护第二份清单：文档里编一个码，这里就红。
func TestDocsErrorCodesExistInSource(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob sources: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no Go sources found")
	}
	for _, item := range docsErrorCodes {
		found := false
		for _, file := range files {
			base := filepath.Base(file)
			// 文档自身（含本测试）当然写着这些码，跳过它们才有意义。
			if base == "docs.go" || strings.HasSuffix(base, "_test.go") {
				continue
			}
			body, readErr := os.ReadFile(file)
			if readErr != nil {
				t.Fatalf("read %s: %v", file, readErr)
			}
			if strings.Contains(string(body), `"`+item.Code+`"`) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("error code %q is documented but no handler ever returns it", item.Code)
		}
	}
}

// 页面声明必须落在 theme.json 上：宿主模板（SSR，首帧即有正文）+
// 模板里挂载公开 L2 组件（只负责填表），组件制品与样式都在清单里。
func TestDocsPageIsAHostTemplatePage(t *testing.T) {
	theme := readPackageJSON(t, "theme.json")
	pages, ok := theme["pages"].([]any)
	if !ok {
		t.Fatal("theme.json: pages is missing")
	}
	var docs map[string]any
	for _, raw := range pages {
		page, _ := raw.(map[string]any)
		if page["path"] == "/docs/oauth2-provider" {
			docs = page
			break
		}
	}
	if docs == nil {
		t.Fatal("theme.json: /docs/oauth2-provider page is missing")
	}
	if docs["access"] != "public" {
		t.Errorf("docs page access = %#v, want \"public\" (integration guide needs no account)", docs["access"])
	}
	if docs["contract"] != "sforum.page.oauth2_provider_docs@1" {
		t.Errorf("docs page contract = %#v", docs["contract"])
	}
	// 首帧必须有正文：正文写在模板里，而不是等客户端组件挂载。
	if docs["template"] != "templates/oauth-docs.html" {
		t.Fatalf("docs page template = %#v, want templates/oauth-docs.html", docs["template"])
	}
	if view, present := docs["view"]; present && view != "" && view != "template" {
		t.Errorf("docs page view = %#v; the body must come from the SSR template", view)
	}

	template := packageFile(t, "templates", "oauth-docs.html")
	for _, needle := range []string{
		"sf-extension-widget",
		`sforum.oauth2-provider.component.docs`,
		"sfp-scope-rows",
		"sfp-endpoint-rows",
		"sfp-error-rows",
	} {
		if !strings.Contains(template, needle) {
			t.Errorf("templates/oauth-docs.html must contain %q", needle)
		}
	}
	// 页头页脚由宿主的外层 shell 渲染（插件页面自动套 plugin shell），
	// 模板里再写一份岛或手写 chrome 都会重复。
	for _, forbidden := range []string{"<sf-navbar", "<sf-footer", "<header", "<nav", "<footer"} {
		if strings.Contains(template, forbidden) {
			t.Errorf("templates/oauth-docs.html must not hand-roll chrome (%q); the host islands own it", forbidden)
		}
	}

	manifest := readPackageJSON(t, "sforum.extension.json")
	declared := false
	for _, raw := range manifest["components"].([]any) {
		item, _ := raw.(map[string]any)
		if item["id"] == "sforum.oauth2-provider.component.docs" {
			declared = true
			if item["l2Component"] != "sforum.oauth2-provider.file.public-docs" {
				t.Errorf("docs component l2Component = %#v", item["l2Component"])
			}
		}
	}
	if !declared {
		t.Fatal("sforum.oauth2-provider.component.docs is mounted by the template but not declared in components[]")
	}

	// 样式必须按组件作用域绑定，否则公开页拿不到它。
	scoped := false
	for _, raw := range manifest["assets"].([]any) {
		asset, _ := raw.(map[string]any)
		if asset["path"] != "frontend/public/docs.css" {
			continue
		}
		scopes, _ := asset["scope"].([]any)
		for _, scope := range scopes {
			if scope == "sforum.oauth2-provider.component.docs" {
				scoped = true
			}
		}
	}
	if !scoped {
		t.Fatal("frontend/public/docs.css must be an asset scoped to sforum.oauth2-provider.component.docs")
	}
}

// 组件只填表：不许再渲染页面骨架（否则又回到「空白卡片 → 整页文字」的闪屏）。
func TestDocsComponentOnlyFillsTables(t *testing.T) {
	script := packageFile(t, "frontend", "public", "docs.mjs")
	for _, needle := range []string{
		"/api/oauth/docs",
		"sfp-scope-rows",
		"sfp-endpoint-rows",
		"sfp-error-rows",
		"sfp-internal-endpoint-rows",
	} {
		if !strings.Contains(script, needle) {
			t.Errorf("docs.mjs must fill the template placeholders: missing %q", needle)
		}
	}
	for _, forbidden := range []string{
		"<main", "<article", "sf-card", "sf-public-page", "<sf-navbar", "<sf-footer", "<!DOCTYPE", "<html",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("docs.mjs must not render page chrome or body (%q); the SSR template owns it", forbidden)
		}
	}
}

// 样式只补 Core 没有的表格排版，颜色必须取 Core token（明暗模式自动跟随）。
func TestDocsStyleUsesCoreTokens(t *testing.T) {
	style := packageFile(t, "frontend", "public", "docs.css")
	for _, needle := range []string{"--sf-public-border", "--sf-public-text-muted", ".sfp-docs__table"} {
		if !strings.Contains(style, needle) {
			t.Errorf("docs.css must build on Core tokens: missing %q", needle)
		}
	}
	if strings.Contains(style, "#fff") || strings.Contains(style, "#000") {
		t.Error("docs.css must not hardcode black/white; use --sf-public-* tokens so dark mode follows the site")
	}
	// 正文列宽必须比 Core 的 --narrow（760px）宽：4 列表格与长代码行在 760px 下会挤成换行。
	if !strings.Contains(style, "max-width: 960px") {
		t.Error("docs.css must widen the content column beyond the 760px narrow default")
	}
}

// 确认页仍是唯一的 stream 路由：文档页是模板页 + 普通 HTTP 数据端点。
func TestStreamRouteServesOnlyConsentPage(t *testing.T) {
	server := testServer(t)

	status, payload, err := server.streamResponse(&pluginwire.RouteStreamOpen{
		RouteId: routeConsentPageID, Path: consentEndpointPath + "?request=sfoq_test",
	})
	if err != nil {
		t.Fatalf("consent stream: %v", err)
	}
	if status != http.StatusOK || !strings.Contains(string(payload), "授权确认") {
		t.Fatalf("consent stream regressed: status=%d", status)
	}

	status, _, err = server.streamResponse(&pluginwire.RouteStreamOpen{
		RouteId: routeDocsID, Path: docsEndpointPath,
	})
	if !errors.Is(err, errUnknownStreamRoute) {
		t.Fatalf("docs route must not be a stream route, got %v", err)
	}
	if status != http.StatusNotFound {
		t.Fatalf("unknown stream route status = %d, want 404", status)
	}
}

// 数据端点走 http 模式：preflight 不该再声明 StreamFollows。
func TestDocsRouteIsPlainHTTP(t *testing.T) {
	server := testServer(t)
	response, err := server.InvokeRoute(context.Background(), testRequest(routeDocsID, map[string]any{}))
	if err != nil {
		t.Fatalf("docs: %v", err)
	}
	if response.GetStreamFollows() {
		t.Fatal("docs route must not declare StreamFollows; it is a JSON document route")
	}
	if response.GetStatusCode() != http.StatusOK || response.GetBody() == nil {
		t.Fatalf("docs route status=%d body=%v", response.GetStatusCode(), response.GetBody() != nil)
	}
}

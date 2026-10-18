package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// 站点公开地址（userinfo 的 avatar 需要绝对地址）。
//
// 宿主 core-safe 投影里的头像 URL 是站点相对路径（/media/avatars/<publicId>），
// 对第三方服务端不可直接访问。插件进程拿不到请求的 Host（宿主路由会过滤 Host 头，
// RequestContext 也不带站点地址），因此按下面的顺序解析公开地址：
//
//  1. 运行时站点地址 site.url：GET {APP_URL}/api/v1/web-options/site.url。这是宿主
//     公开选项接口，site.url 未在后台覆盖时由宿主的 APP_URL 动态兜底，覆盖时以
//     后台值为准（与宿主自己的 OAuth 回调地址逻辑一致）。
//  2. 进程环境里的 APP_URL：宿主启动插件时继承环境，部署期至少保证它是可访问地址。
//  3. 都拿不到时保留相对路径（尽力而为，不伪造域名）。
//
// 结果缓存 5 分钟：站点地址几乎不变，避免每次 userinfo 都多一次网络往返。

const (
	publicBaseURLCacheTTL = 5 * time.Minute
	publicBaseURLTimeout  = 2 * time.Second
	publicBaseURLMaxBody  = 64 << 10
	siteURLOptionPath     = "/api/v1/web-options/site.url"
)

// publicBaseURL 返回当前站点公开地址（无尾斜杠）；解析失败时返回空串。
func (s *oauthServer) publicBaseURL(ctx context.Context) string {
	s.baseURLMu.Lock()
	if s.baseURLFetchedAt.After(time.Now().Add(-publicBaseURLCacheTTL)) {
		cached := s.baseURL
		s.baseURLMu.Unlock()
		return cached
	}
	s.baseURLMu.Unlock()

	resolved := resolvePublicBaseURL(ctx)

	s.baseURLMu.Lock()
	s.baseURL = resolved
	s.baseURLFetchedAt = time.Now()
	s.baseURLMu.Unlock()
	return resolved
}

// resolvePublicBaseURL 先读运行时 site.url，再回退 APP_URL。
func resolvePublicBaseURL(ctx context.Context) string {
	appURL := normalizeBaseURL(os.Getenv("APP_URL"))
	if siteURL := fetchSiteURLOption(ctx, appURL); siteURL != "" {
		return siteURL
	}
	return appURL
}

// fetchSiteURLOption 通过宿主公开选项接口读取 site.url。
// 失败、超时或返回非法地址都返回空串，由调用方回退 APP_URL。
func fetchSiteURLOption(ctx context.Context, appURL string) string {
	if appURL == "" {
		return ""
	}
	requestCtx, cancel := context.WithTimeout(ctx, publicBaseURLTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, appURL+siteURLOptionPath, nil)
	if err != nil {
		return ""
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, publicBaseURLMaxBody))
	if err != nil {
		return ""
	}
	var payload struct {
		Data struct {
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return normalizeBaseURL(payload.Data.Value)
}

// normalizeBaseURL 只接受 http/https 绝对地址，去掉 query / fragment / 尾斜杠。
func normalizeBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/")
}

// profilePathPrefix 是站点公开个人主页路由前缀（apps/web/app/pages/u/[username].vue）。
// userinfo 的 profileUrl 与 GitHub 的 html_url 对齐：第三方点一下就能回到站内主页。
const profilePathPrefix = "/u/"

// profileURL 返回站内个人主页的绝对地址。站点公开地址解析不出来时返回空串：
// 相对路径对第三方服务端不可用，猜域名更糟（与 avatar 的绝对化策略一致）。
func (s *oauthServer) profileURL(ctx context.Context, username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return ""
	}
	base := s.publicBaseURL(ctx)
	if base == "" {
		return ""
	}
	return base + profilePathPrefix + url.PathEscape(username)
}

// isAbsoluteURL 判断是否已经是可直接访问的地址（http/https/data 等）。
func isAbsoluteURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	return err == nil && parsed.IsAbs()
}

// absoluteURL 把宿主给的相对地址补成绝对地址。baseURL 为空或非法时原样返回，
// 不猜测域名；已绝对的值（含 data URL）原样返回。
func absoluteURL(baseURL, value string) string {
	value = strings.TrimSpace(value)
	if value == "" || isAbsoluteURL(value) {
		return value
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return value
	}
	reference, err := url.Parse(value)
	if err != nil {
		return value
	}
	return base.ResolveReference(reference).String()
}

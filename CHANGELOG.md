# Changelog

本项目遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与
[语义化版本](https://semver.org/lang/zh-CN/)。

## [1.0.0] - 2026-10-15

### Added

- OAuth2 授权服务器：授权码 + PKCE（S256 / plain）、刷新令牌轮换、令牌撤销（RFC 7009）、
  令牌内省（RFC 7662）、userinfo。
- 授权确认页（`mode: stream` 自包含 HTML）与 `require_consent` 策略；已授权应用列表与撤销。
- 应用管理：管理员管理全部应用（`sforum.oauth2-provider.manage_clients`），用户可自助创建、
  编辑、重置密钥、删除；一次性明文密钥只在创建/重置响应中出现。
- 管理端面板（Admin Micro-frontend API v1，免构建 ESM）：列表卡片 + 新建/编辑模态、
  一次性密钥视图、危险操作二次确认、窄屏堆叠、暗色跟随宿主主题。
- 存储：`own_schema` 租约（`oauth_clients` / `oauth_requests` / `oauth_tokens` / `oauth_consents`），
  密钥与令牌只存 SHA-256；启用幂等建表，停用/卸载保留数据。
- 设置项：令牌有效期、自助创建开关、是否必须确认授权、允许的权限范围，均可一键恢复默认值。
- 契约测试：与宿主相同的 JSON Schema 校验每个 handler 的真实响应，防止响应契约漂移。

[1.0.0]: https://github.com/zhuchunshu/sforum-oauth2-provider/releases/tag/v1.0.0

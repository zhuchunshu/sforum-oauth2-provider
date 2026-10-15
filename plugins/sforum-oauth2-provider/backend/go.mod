module github.com/zhuchunshu/sforum-oauth2-provider/backend

go 1.26.9

require (
	github.com/jackc/pgx/v5 v5.10.0
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2
	github.com/zhuchunshu/sforum/apps/api v0.0.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.8.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.22 // indirect
	github.com/oklog/run v1.1.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.2 // indirect
)

// SForum 的 apps/api 尚未作为 Go module 发布（tag 形如 v3.x，不是 apps/api/vX），
// 因此插件通过本地检出解析 SDK。默认约定：本仓库与 SForum 检出目录同级：
//
//   Code/github/sforum-oauth2-provider                      ← 本仓库（仓库根就是外部源根）
//   Code/github/sforum-oauth2-provider/plugins/…             ← 扩展包本体
//   Code/github/SForum                                       ← 框架检出
//
// CI 会用 `go mod edit -replace` 覆盖为它自己的检出路径（见 .github/workflows）。
replace github.com/zhuchunshu/sforum/apps/api => ../../../../SForum/apps/api

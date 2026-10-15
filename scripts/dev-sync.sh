#!/usr/bin/env bash
# 把本仓库同步到 SForum 的本地开发插件目录，便于在开发站点里启用调试。
# 用法：scripts/dev-sync.sh [目标目录]
#   默认目标：<SForum>/extensions/dev/plugins/sforum-oauth2-provider
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SFORUM_DIR="${SFORUM_DIR:-$(cd "$ROOT/.." && pwd)/SForum}"
TARGET="${1:-$SFORUM_DIR/extensions/dev/plugins/sforum-oauth2-provider}"

if [ ! -d "$SFORUM_DIR/apps/api" ]; then
  echo "找不到 SForum 检出目录：$SFORUM_DIR（可用 SFORUM_DIR=… 指定）" >&2
  exit 1
fi

echo "==> 构建并刷新摘要"
"$ROOT/scripts/prepare-package.sh" "$ROOT"

echo "==> 同步到 $TARGET"
mkdir -p "$TARGET"
# 排除项用 / 前缀锚定到仓库根，避免误伤 frontend/admin/dist。
rsync -a --delete \
  --exclude '/.git/' --exclude '/.build/' --exclude '/dist/' --exclude '/scripts/' \
  --exclude '/.github/' --exclude '/backend/*.go' --exclude '/backend/go.sum' \
  "$ROOT/" "$TARGET/"

cat <<'TIP'
完成。接下来在 SForum 里：
  1. 重启 API 让宿主重新扫描外部插件根：./scripts/api-dev.sh
  2. 后台「插件管理」升级/启用 OAuth2 Provider（走一次 super_admin 信任确认）
注意：包内容变化会生成新的候选制品，必须重新确认信任；同版本号只改内容时用「禁用 → 升级 → 启用」。
TIP

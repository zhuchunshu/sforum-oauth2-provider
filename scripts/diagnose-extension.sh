#!/usr/bin/env bash
# 插件「起不来」的取证脚本：一条命令打印宿主账本与运行时实况。
#
# 背景：UI 上只弹一句错误（例如 must be drained before transition），真实原因
# 分散在 lifecycle 账本、composed boundary 出版物、runtime lease 与进程树里。
# 本脚本把这些拼成一份可读报告，全部只读，不写任何状态。
#
# 用法：
#   scripts/diagnose-extension.sh [extension-id]
#   EXTENSION_ID=sforum.oauth2-provider SFORUM_DIR=~/Code/github/SForum scripts/diagnose-extension.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SFORUM_DIR="${SFORUM_DIR:-$(cd "$ROOT/.." && pwd)/SForum}"
EXT="${1:-${EXTENSION_ID:-sforum.oauth2-provider}}"
OPS_LIMIT="${OPS_LIMIT:-8}"

ENV_FILE="$SFORUM_DIR/.env"
if [ ! -f "$ENV_FILE" ]; then
  echo "找不到宿主 .env：$ENV_FILE（可用 SFORUM_DIR=/path/to/SForum 指定）" >&2
  exit 1
fi

# 只解析 DATABASE_URL，不回显口令。
DB_URL="$(sed -nE 's/^DATABASE_URL=(.*)$/\1/p' "$ENV_FILE" | head -1 | tr -d '"')"
if [ -z "$DB_URL" ]; then
  echo ".env 里没有 DATABASE_URL" >&2
  exit 1
fi

# 本机一般没有 psql 客户端，dev 库跑在容器里：优先 psql，其次 docker exec。
sql() {
  local query="$1"
  if command -v psql >/dev/null 2>&1; then
    psql "$DB_URL" -v ON_ERROR_STOP=0 -c "$query"
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    # 从 URL 拆出 user/db/口令/映射端口，端口用于定位容器。
    local user db pass port container
    user="$(printf '%s' "$DB_URL" | sed -nE 's#^postgres(ql)?://([^:/]+).*#\2#p')"
    db="$(printf '%s' "$DB_URL" | sed -nE 's#^postgres(ql)?://[^/]*/([^?]+).*#\2#p')"
    pass="$(printf '%s' "$DB_URL" | sed -nE 's#^postgres(ql)?://[^:]*:([^@]*)@.*#\2#p')"
    port="$(printf '%s' "$DB_URL" | sed -nE 's#.*@[^:/]+:([0-9]+).*#\1#p')"
    container="$(docker ps --filter "publish=$port" --format '{{.Names}}' | head -1)"
    [ -n "$container" ] || container="$(docker ps --format '{{.Names}}' | grep -i postgres | head -1 || true)"
    if [ -z "$container" ]; then
      echo "既没有 psql，也找不到暴露 $port 的 postgres 容器" >&2
      return 1
    fi
    # PGPASSWORD 只进容器环境，绝不打印。
    docker exec -e PGPASSWORD="$pass" "$container" psql -U "$user" -d "$db" \
      -v ON_ERROR_STOP=0 -c "$query"
    return
  fi
  echo "需要 psql 或 docker 才能查库" >&2
  return 1
}

section() { printf '\n===== %s =====\n' "$1"; }

section "扩展主记录"
sql "select id, type, status, active_version_id, staged_version_id, source, is_system, updated_at
       from extensions where id = '$EXT';"

section "最近 $OPS_LIMIT 个 lifecycle operation（含压平的报错）"
sql "select id, operation, state, terminal_result, current_step_id, error_code,
             left(regexp_replace(coalesce(error_message,''), '\s+', ' ', 'g'), 220) as error_message,
             created_at, completed_at
        from extension_lifecycle_operations
       where extension_id = '$EXT'
       order by id desc limit $OPS_LIMIT;"

section "开放中的 lifecycle operation（应为 0 行）"
sql "select id, extension_id, operation, state, updated_at
       from extension_lifecycle_operations
      where completed_at is null
       order by id;"

section "runtime lease：active 行（插件真正跑着的是谁）"
sql "select id, runtime_instance_id, extension_version_id,
             left(package_digest, 12) as package_digest, status, last_heartbeat_at, lease_expires_at
        from extension_database_runtime_leases
       where status = 'active'
       order by id desc limit 12;"

section "本扩展最近的 runtime lease"
sql "select id, runtime_instance_id, extension_version_id,
             left(package_digest, 12) as package_digest, status, issued_at, revoked_at
        from extension_database_runtime_leases
       where extension_id = '$EXT'
       order by id desc limit 6;"

section "权威 runtime 出版物成员（决定重启后会拉起谁）"
sql "select publication_revision, extension_id, extension_version_id, left(package_digest, 12) as package_digest
        from plugin_runtime_publication_members
       where publication_revision = (select max(publication_revision) from plugin_runtime_publication_members)
       order by extension_id;"
sql "select revision, member_count, reason, created_at
        from plugin_runtime_publications
       order by revision desc limit 3;"

section "悬挂的 composed boundary（commit_marker=f 即半提交，是重启复现老实例的根因）"
sql "select operation, operation_id, publication_mode, commit_marker, committed_at,
             left(coalesce(source_package_digest,''), 12) as source_digest,
             left(coalesce(target_package_digest,''), 12) as target_digest,
             updated_at
        from extension_lifecycle_publications
       where source_extension_id = '$EXT' or target_extension_id = '$EXT'
          or operation_id in (select id from extension_lifecycle_operations where extension_id = '$EXT')
       order by operation_id desc limit $OPS_LIMIT;"

section "悬挂的 registry publication（transaction_state=source 且无 published_at = 注册集未撤销）"
sql "select operation_id, step_id, position, transaction_state, publication_mode, published_at,
             left(coalesce(source_package_digest,''), 12) as source_digest,
             left(coalesce(target_package_digest,''), 12) as target_digest
        from extension_lifecycle_registry_publications
       where target_extension_id = '$EXT' or source_extension_id = '$EXT'
       order by operation_id desc limit $OPS_LIMIT;"

section "state publication（status/active_version 的账本轨迹）"
sql "select operation_id, step_id, transaction_state, source_status, target_status,
             source_active_version_id, target_active_version_id, published_at
        from extension_lifecycle_state_publications
       where extension_id = '$EXT'
       order by operation_id desc limit $OPS_LIMIT;"

section "进程树：backend/plugin 与其父进程"
if command -v ps >/dev/null 2>&1; then
  ps -eo pid,ppid,etime,cmd 2>/dev/null | grep -E "storage/extensions/.*backend/plugin" | grep -v grep || echo "（没有插件子进程）"
  echo "-- live sforum-api --"
  pgrep -a sforum-api 2>/dev/null || echo "（sforum-api 不在运行）"
fi

section "可读性探针"
if command -v curl >/dev/null 2>&1; then
  # 本地探针必须绕开代理，否则 127.0.0.1 会被 clash 之类拦掉。
  curl -s --noproxy '*' -m 5 http://127.0.0.1:8080/api/v1/ready || echo "（ready 探针失败：API 未起？）"
  echo
  printf '本扩展公共路由 code=%s\n' \
    "$(curl -s --noproxy '*' -m 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/oauth/csrf 2>/dev/null || echo 000)"
fi

printf '\n判读提示：\n'
cat <<'TXT'
  - 没有 active lease、且权威出版物里不含本扩展 => 可以「启用」，新实例会按当前制品冷启动。
  - 出版物里含本扩展但 extensions.status=disabled => 半提交撤销边界；用宿主带外恢复摘除成员：
      cd $SFORUM_DIR/apps/api && DATABASE_URL=<.env 里的值> go run ./cmd/sforum extension disable <extension-id>
    （PublishPluginRuntimeRecoveryTx 会追加一个 recovery revision 并让协调器 drain 掉残留实例。）
  - 报错 "must be drained before transition" => 有残留实例挡路，不是包的问题；别反复点启用。
  - 报错 "service registry: ... exact fence conflict" => 补偿路径撞上残留实例的注册快照。
TXT

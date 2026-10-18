#!/usr/bin/env bash
# 构建本机平台的插件二进制、刷新包摘要并跑一遍宿主契约校验。
# 用法：scripts/prepare-package.sh [extension-root]（默认 plugins/sforum-oauth2-provider）
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PKG="${1:-$ROOT/plugins/sforum-oauth2-provider}"
SFORUM_DIR="${SFORUM_DIR:-$(cd "$ROOT/.." && pwd)/SForum}"

if [ ! -d "$SFORUM_DIR/apps/api" ]; then
  echo "找不到 SForum 检出目录：$SFORUM_DIR" >&2
  echo "可用 SFORUM_DIR=/path/to/SForum 指定，或把本仓库与 SForum 放在同一父目录下。" >&2
  exit 1
fi

echo "==> 构建 backend/plugin（与宿主 dev watcher 同参：CGO_ENABLED=0，否则两边产物字节不同、digest 永不收敛）"
(cd "$PKG/backend" && CGO_ENABLED=0 go build -trimpath -o "$PKG/backend/plugin" ./)

# 宿主 validateComponentUpgrade 的硬约束：同一 components[].contractVersion 下，
# 被引用的 props/result schema、L2 前端制品或 SSR 模板一旦变化，升级会被判
# 「changed without a contract version」而整条 lifecycle 失败。这里在构建前
# 比对上一次成功构建的引用摘要，逼作者先改契约版本，避免把失败留到宿主升级时。
STATE="$ROOT/.build/component-contract-state.json"
echo "==> 检查组件契约版本（components[].contractVersion）"
python3 - "$PKG" "$STATE" <<'PY'
import hashlib, json, os, sys

root, state_path = sys.argv[1], sys.argv[2]
with open(os.path.join(root, "sforum.extension.json"), encoding="utf-8") as handle:
    manifest = json.load(handle)

paths = {entry.get("id"): entry.get("path") for entry in manifest.get("packageFiles", [])}
current = {}
for component in manifest.get("components", []):
    refs = {}
    for key in ("propsSchema", "resultSchema", "l2Component", "ssrTemplate"):
        reference = (component.get(key) or "").strip()
        # 引用可能带版本后缀（sforum.x.props@1），packageFiles[].id 不带。
        relative = paths.get(reference) or paths.get(reference.rsplit("@", 1)[0])
        if not relative:
            continue
        with open(os.path.join(root, relative), "rb") as handle:
            refs[key] = hashlib.sha256(handle.read()).hexdigest()
    current[component.get("id", "")] = {
        "contractVersion": (component.get("contractVersion") or "").strip(),
        "refs": refs,
    }

if not os.path.exists(state_path):
    print("no previous build state; contract check skipped (state written at the end)")
    sys.exit(0)

with open(state_path, encoding="utf-8") as handle:
    previous = json.load(handle)

failures = []
for component_id, entry in current.items():
    before = previous.get(component_id)
    if not before:
        continue
    if entry["refs"] != before.get("refs", {}) and entry["contractVersion"] == before.get("contractVersion"):
        failures.append(component_id)

if failures:
    print("组件契约版本未变但被引用制品已改变：" + ", ".join(failures), file=sys.stderr)
    print(
        "请先把 sforum.extension.json 里对应 components[].contractVersion 提升一个版本"
        "（例如 @19 → @20），否则宿主升级会以「changed without a contract version」失败。",
        file=sys.stderr,
    )
    sys.exit(1)
print("component contract versions are consistent with referenced artifacts")
PY

# Host 的 digest 命令只刷新 digest，不会重算 assets[].integrity（SRI）。
# CSS/JS 一改而 integrity 没跟着变，包会被判 invalid，而 CLI 报的却是另一个文件的
# digest mismatch，极易误判。因此这里先按文件重算 integrity，再让 digest 刷新摘要。
echo "==> 重算 assets[].integrity（SRI）"
python3 - "$PKG" <<'PY'
import base64, hashlib, json, os, sys

root = sys.argv[1]
manifest_path = os.path.join(root, "sforum.extension.json")
with open(manifest_path, encoding="utf-8") as handle:
    manifest = json.load(handle)

changed = []
for asset in manifest.get("assets", []):
    relative = asset.get("path", "")
    if not relative:
        continue
    with open(os.path.join(root, relative), "rb") as handle:
        body = handle.read()
    integrity = "sha256-" + base64.b64encode(hashlib.sha256(body).digest()).decode()
    if asset.get("integrity") != integrity:
        asset["integrity"] = integrity
        changed.append(relative)

if changed:
    with open(manifest_path, "w", encoding="utf-8") as handle:
        json.dump(manifest, handle, ensure_ascii=False, indent=2, sort_keys=True)
        handle.write("\n")
    print("updated integrity: " + ", ".join(changed))
else:
    print("integrity already current")
PY

echo "==> 刷新包摘要（extension digest --write ×2：首写会变更 manifest 自身，第二轮收敛后才可校验）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension digest --write "$PKG")
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension digest --write "$PKG")

echo "==> 校验清单（extension validate）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension validate "$PKG")

echo "==> 宿主契约检查（extension test）"
(cd "$SFORUM_DIR/apps/api" && go run ./cmd/sforum extension test "$PKG")

# 只有全部校验通过后才记录本次的组件引用摘要，失败构建不会污染下一次比对。
python3 - "$PKG" "$STATE" <<'PY'
import hashlib, json, os, sys

root, state_path = sys.argv[1], sys.argv[2]
with open(os.path.join(root, "sforum.extension.json"), encoding="utf-8") as handle:
    manifest = json.load(handle)

paths = {entry.get("id"): entry.get("path") for entry in manifest.get("packageFiles", [])}
state = {}
for component in manifest.get("components", []):
    refs = {}
    for key in ("propsSchema", "resultSchema", "l2Component", "ssrTemplate"):
        reference = (component.get(key) or "").strip()
        # 引用可能带版本后缀（sforum.x.props@1），packageFiles[].id 不带。
        relative = paths.get(reference) or paths.get(reference.rsplit("@", 1)[0])
        if not relative:
            continue
        with open(os.path.join(root, relative), "rb") as handle:
            refs[key] = hashlib.sha256(handle.read()).hexdigest()
    state[component.get("id", "")] = {
        "contractVersion": (component.get("contractVersion") or "").strip(),
        "refs": refs,
    }

os.makedirs(os.path.dirname(state_path), exist_ok=True)
with open(state_path, "w", encoding="utf-8") as handle:
    json.dump(state, handle, ensure_ascii=False, indent=2, sort_keys=True)
    handle.write("\n")
PY

echo "完成：$PKG"

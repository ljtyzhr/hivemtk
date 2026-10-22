#!/usr/bin/env bash
# =============================================================
# HiveMtk 用户端 发布脚本
#
# 部署形态（2026-09 起）：默认纯本地。
#   - 本地（宿主机）：user-server 二进制 + PG + Redis + LLM 推理栈
#   - 远端：只有显式给出 DEPLOY_HOST 才会执行前端推送那一段。
#     历史上那台云端主机（含反向代理层与 frps）已随域名一起停用，
#     所以这里刻意不留任何默认目标机 —— 留一个就是让发布脚本往黑洞里 rsync。
#   - API 路径（本地）：浏览器 → http://127.0.0.1:8204/api/…
#
# 部署内容：
#   - user-server → 本地二进制（go build → nohup 重启，127.0.0.1:8204）
#   - user-web 前端 → $DEPLOY_HOST:$WEBROOT_USERWEB/$WEBROOT_USERWEB_SUBDIR（需显式 DEPLOY_HOST）
#   - embed-sdk     → $DEPLOY_HOST:$EMBED_SDK_WEBROOT（默认 $WEBROOT_USERWEB/embed-sdk-dist，同上）
#
# 用法:
#   ./deploy-user.sh --api-only                          # 纯本地：只重启本地 user-server
#   DEPLOY_HOST=<你自己的主机> ./scripts/deploy-user.sh   # 全量：本地 + 推前端到该主机
#   ./deploy-user.sh --web-only                          # 只推前端（同样必须给 DEPLOY_HOST）
#   ./deploy-user.sh --skip-build                        # 跳过本地前端构建
#   ./deploy-user.sh --反向代理层-only                    # 只更新云端 同源托管配置
#   ./deploy-user.sh --dry-run                           # 仅打印命令
# =============================================================
set -euo pipefail

# ---------- 路径 ----------
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# ---------- 日志 ----------
log()     { echo "[$(date +'%H:%M:%S')] $*"; }
log_warn(){ echo "[$(date +'%H:%M:%S')] WARN: $*" >&2; }
log_err() { echo "[$(date +'%H:%M:%S')] ERROR: $*" >&2; }
die()     { log_err "$*"; exit 1; }

# ---------- 可覆盖变量 ----------
DEPLOY_USER="${DEPLOY_USER:-root}"
# 刻意无默认值：远端步骤必须显式指定目标机（见 preflight 里的 need_deploy_host）
DEPLOY_HOST="${DEPLOY_HOST:-}"
REMOTE="ssh $DEPLOY_USER@$DEPLOY_HOST"

WEBROOT_USERWEB="${WEBROOT_USERWEB:-/www/wwwroot/hivemtk/user-web}"
# 站点根下的子目录：历史布局为 dist/；生产统一栈的站点根本身就是 dist 内容，
# 用 WEBROOT_USERWEB_SUBDIR= 置空即可平铺推送（必须用 - 而非 :-，后者会把空值当未设置）。
WEBROOT_USERWEB_SUBDIR="${WEBROOT_USERWEB_SUBDIR-dist}"
WEBROOT_USERWEB_DIST="${WEBROOT_USERWEB}/${WEBROOT_USERWEB_SUBDIR}"
# embed-sdk 产物落点：历史与 user-web 同根；生产统一栈由 user-server 目录托管。
EMBED_SDK_WEBROOT="${EMBED_SDK_WEBROOT:-$WEBROOT_USERWEB/embed-sdk-dist}"
USER_SERVER_PORT="${USER_SERVER_PORT:-8204}"

REMOTE_DEPLOY_DIR="${REMOTE_DEPLOY_DIR:-/www/wwwroot/hivemtk}"
REMOTE_GIT_REPO="${REMOTE_GIT_REPO:-git@gitee.com:xhpmayun/hivemtk.git}"
REMOTE_GIT_BRANCH="${REMOTE_GIT_BRANCH:-master}"

# 发布后的健康检查打本地端口：域名停用后，"发布是否成功"这件事只有本地这一处能自证。
# 探针必须是 /health（router.go 的四层探测：应用/DB/Redis/推理栈）——/api/health 是空探针
# （systemInfoCtrl.Health 恒回 status:ok + HTTP 200），依赖挂了也照样 200，探不出发布失败。
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:${USER_SERVER_PORT}/health}"

SKIP_HEALTHCHECK="${SKIP_HEALTHCHECK:-}"
SKIP_BUILD="${SKIP_BUILD:-}"
SKIP_BACKUP="${SKIP_BACKUP:-}"   # SKIP_BACKUP=1 跳过发布前 pg_dump（逃生门，见 predeploy_backup）
DRY_RUN="${DRY_RUN:-}"
MODE="${MODE:-all}"

# ---------- 参数解析 ----------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --web-only)     MODE="web"; shift ;;
    --api-only)     MODE="api"; shift ;;
    --skip-build)   SKIP_BUILD=1; shift ;;
    --反向代理层-only)   MODE="反向代理层"; shift ;;
    --dry-run)      DRY_RUN=1; shift ;;
    -h|--help)
      # 按标记取"用法"段，不写死行号（改注释就不会让 --help 悄悄截半）
      sed -n '/^# 用法:/,/^# =\{10,\}/p' "$0"
      exit 0
      ;;
    *) die "未知参数: $1" ;;
  esac
done

# ---------- 发布并发锁 ----------
# mkdir 是原子操作：抢到即持锁，失败即有人在发布。不用 flock(1) —— macOS 无此命令，
# 脚本要在开发机（macOS）与部署机（Linux）两边都能跑。
DEPLOY_LOCK_DIR="$ROOT/.deploy.lock"
if ! mkdir "$DEPLOY_LOCK_DIR" 2>/dev/null; then
  die "另一发布在进行中（锁目录已存在：${DEPLOY_LOCK_DIR}）；确认没有并发发布后删除该目录再跑"
fi
trap 'rmdir "$DEPLOY_LOCK_DIR" 2>/dev/null || true' EXIT
# 注意：变量后紧跟全角括号时必须用 ${} 显式定界，否则 bash 会把全角字符并进变量名（set -u 下报 unbound）
log "已获取发布锁：${DEPLOY_LOCK_DIR}（异常退出由 trap 清理）"

# ---------- 本地命令 ----------
run() {
  if [[ -n "$DRY_RUN" ]]; then echo "[dry-run] $*"; return 0; fi
  bash -c "set -euo pipefail; $*"
}

# ---------- 远端命令 ----------
run_remote() {
  local desc="$1"; shift
  if [[ -n "$DRY_RUN" ]]; then
    echo "[dry-run|remote] $desc -> $REMOTE $*"
    return 0
  fi
  $REMOTE "$@"
}

# ---------- 远端步骤判定 ----------
# web / all 两种模式含"推前端到远端"，必须显式给 DEPLOY_HOST；api 模式纯本地，不需要。
mode_needs_remote() {
  case "$MODE" in
    web|all) return 0 ;;
    *) return 1 ;;
  esac
}

need_deploy_host() {
  if mode_needs_remote && [[ -z "$DRY_RUN" && -z "$DEPLOY_HOST" ]]; then
    die "缺少 DEPLOY_HOST：本次模式（${MODE}）要把前端推到远端，而本脚本不再内置默认目标机。纯本地发布请用 --api-only；确实要推远端请显式指定，例如 DEPLOY_HOST=10.0.0.5 ./scripts/deploy-user.sh"
  fi
}

# ---------- 平台链路开关（漂移检查用） ----------
# 优先取已导出的环境变量，其次从仓库根 .env 抽（脚本自身不 source .env，
# 避免 PORT 等键污染本脚本的探测变量；启动子进程时才 source）。
platform_enabled() {
  local v="${PLATFORM_ENABLED:-}"
  if [[ -z "$v" && -f "$ROOT/.env" ]]; then
    v="$(grep -E '^PLATFORM_ENABLED=' "$ROOT/.env" 2>/dev/null | tail -1 | cut -d= -f2- | tr -d '[:space:]' || true)"
    v="${v#\"}"; v="${v%\"}"; v="${v#\'}"; v="${v%\'}"
  fi
  [[ "$v" == "true" ]]
}

# ---------- 预检 ----------
preflight() {
  log "########## 预检 ##########"

  for cmd in ssh scp rsync git go; do
    command -v "$cmd" >/dev/null 2>&1 || die "本地命令缺失: $cmd"
  done
  log "  本地命令: ok"

  need_deploy_host

  if [[ -n "$DRY_RUN" ]]; then
    log "  跳过 SSH/本地预检（dry-run）"
    return 0
  fi

  if mode_needs_remote; then
    $REMOTE "echo ok" >/dev/null 2>&1 || die "无法 SSH 到 $DEPLOY_USER@$DEPLOY_HOST"
    log "  SSH 连通: ok"
  else
    log "  本次无远端步骤，跳过 SSH 检查"
  fi

  # 本地 docker（PG + Redis 在本地宿主机运行）
  command -v docker >/dev/null 2>&1 || die "本地命令缺失: docker（数据层 PG+Redis 需要）"
  log "  本地 docker: ok"

  # 本地 go（user-server 二进制编译需要）
  command -v go >/dev/null 2>&1 || die "本地命令缺失: go（user-server 编译需要）"
  log "  本地 go: ok"

  # 本地 .env（user-server 与 docker compose 均依赖）
  if [[ -f "$ROOT/.env" ]]; then
    log "  本地 .env: ok"
  else
    log_warn "本地 $ROOT/.env 不存在，user-server 启动可能失败（DB 密码等缺失）"
  fi
}

# ---------- 本地构建前端 ----------
build_web() {
  [[ "$SKIP_BUILD" == "1" ]] && { log "跳过前端构建（--skip-build）"; return 0; }

  log "########## 构建 user-web 前端 ##########"
  run "cd user-web && npm ci --no-audit --no-fund && npm run build"
  [[ -d user-web/dist ]] || die "user-web 构建失败：未产出 dist/ 目录"
  log "  user-web 构建完成"

  log "########## 构建 embed-sdk ##########"
  run "cd embed-sdk && npm ci --no-audit --no-fund && npm run build"
  [[ -d embed-sdk/dist ]] || die "embed-sdk 构建失败：未产出 dist/ 目录"
  log "  embed-sdk 构建完成"
}

# ---------- 推 dist 到远端 ----------
push_web() {
  log "########## 推送前端资源 ##########"
  run_remote "创建远端目录" "mkdir -p $WEBROOT_USERWEB_DIST $EMBED_SDK_WEBROOT"

  log "==> 推送 user-web/dist -> $WEBROOT_USERWEB_DIST/"
  if command -v rsync >/dev/null 2>&1; then
    run "rsync -az --delete --exclude='.user.ini' user-web/dist/ $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB_DIST/"
  else
    run_remote "清空远端 dist" "find $WEBROOT_USERWEB_DIST -mindepth 1 -name '.user.ini' -prune -o -mindepth 1 -delete"
    run "scp -r user-web/dist/. $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB_DIST/"
  fi

  log "==> 推送 embed-sdk/dist -> $EMBED_SDK_WEBROOT/"
  if command -v rsync >/dev/null 2>&1; then
    run "rsync -az --delete embed-sdk/dist/ $DEPLOY_USER@$DEPLOY_HOST:$EMBED_SDK_WEBROOT/"
  else
    run_remote "清空远端 embed-sdk-dist" "find $EMBED_SDK_WEBROOT -mindepth 1 -delete"
    run "scp -r embed-sdk/dist/. $DEPLOY_USER@$DEPLOY_HOST:$EMBED_SDK_WEBROOT/"
  fi

  log "  前端资源推送完成"
}

# ---------- 本地健康探针（deploy_api 内层与发布后 healthcheck 共用） ----------
# 判定分两步，缺一不可：
#   1) 打 $HEALTH_URL（/health，四层探测）并抓响应体；
#   2) grep body —— 不能只看 HTTP 状态码：/health 的降级分支走
#      response.ErrorWithBusinessCode，HTTP 仍是 200、body code=50301、data.status=degraded；
#      只看状态码会把"降级"判成"发布成功"。
# 反过来只 grep "status":"ok" 也不够：降级响应里 checks.database 等内嵌字段同样带
# "status":"ok"，所以必须再显式排除 degraded 才算通过。
probe_ok() {
  local url="$1"
  local body=''
  # 注意：set -euo pipefail 下 curl 失败（连接被拒/404）不能让整链非零退出，用 || true 兜住。
  body=$(curl -fsS "$url" 2>/dev/null || true)
  [[ -n "$body" ]] || return 1
  # Go 侧 gin 回的是紧凑 JSON（"status":"ok"，无空格）；正则里留 [[:space:]]* 是为了
  # 中间层若做了美化/压缩重排也不至于误判。
  grep -Eq '"status"[[:space:]]*:[[:space:]]*"ok"' <<<"$body" || return 1
  if grep -Eq '"status"[[:space:]]*:[[:space:]]*"degraded"' <<<"$body"; then
    return 1
  fi
  return 0
}

# ---------- 发布前备份（kill 旧进程之前；参数与 Makefile db-backup 同款） ----------
# 新版本启动前可能跑迁移/换口径，先落一份 pg_dump：备份失败必须挡住发布（die），
# 逃生门 SKIP_BACKUP=1 跳过（例如纯配置改动、或磁盘临时不够时人工确认后放行）。
predeploy_backup() {
  if [[ -n "$SKIP_BACKUP" ]]; then
    log "跳过发布前备份（SKIP_BACKUP=1）"
    return 0
  fi
  log "########## 发布前备份 PG ##########"
  run "cd \"$ROOT\" && mkdir -p backups && set -a && [ -f .env ] && source .env; set +a && docker exec mtk-postgres pg_dump -U \"\${POSTGRES_USER:-admin}\" \"\${USER_DB_NAME:-user_db}\" > backups/predeploy_\$(date +%Y%m%d_%H%M%S).sql" \
    || die "发布前 pg_dump 失败，已阻止发布（先核 docker compose 里的 mtk-postgres 在不在；确认无碍需放行时设 SKIP_BACKUP=1）"
  # 备份后把文件大小亮出来：0 字节/几百字节的“备份”多半是 pg_dump 没真跑成
  run "cd \"$ROOT\" && ls -la backups | tail -n 3"
  log "  发布前备份完成（backups/predeploy_*.sql）"
}

# ---------- 发布版本指纹（健康通过后写入，供 post_deploy_check.sh 展示） ----------
write_deploy_version() {
  local desc
  desc="$(git -C "$ROOT" describe --always --dirty 2>/dev/null || git -C "$ROOT" rev-parse --short HEAD)"
  echo "${desc} （$(date -u +%FT%TZ)）" > "$ROOT/user-server/.deploy-version"
  log "  部署版本已写入：user-server/.deploy-version（${desc}）"
}

# ---------- 本地部署 user-server（宿主机二进制） ----------
deploy_api() {
  log "########## 部署 user-server（本地宿主机二进制）##########"

  # 0) 确保数据层（PG + Redis）在本地运行
  log "==> 检查本地数据层（PG + Redis）"
  run "cd \"$ROOT\" && docker compose up -d"
  log "  本地 PG(127.0.0.1:8202) + Redis(127.0.0.1:8203) 已就绪"

  # 0.5) 发布前备份（在 kill 旧进程与换二进制之前，见 predeploy_backup）
  predeploy_backup

  # 1) 编译 user-server 二进制（本地宿主机原生）
  #    编译到 .new：build 失败时不碰现役 bin/user-server（文件层也安全）；
  #    成功后现役 mv 到 .prev（最低成本回滚点）再让 .new 上位 —— mv 是同目录 rename，
  #    原子替换，消除“正在运行的旧进程占着文件 → ETXTBSY / 写一半损坏现役二进制”。
  log "==> 编译 user-server 二进制"
  run "cd \"$ROOT/user-server\" && mkdir -p bin && CGO_ENABLED=0 go build -o bin/user-server.new ./cmd/api"
  if [[ -z "$DRY_RUN" ]]; then   # dry-run 不真编译，跳过产物存在性检查
    [[ -f "$ROOT/user-server/bin/user-server.new" ]] || die "user-server 编译失败"
  fi
  run "cd \"$ROOT/user-server/bin\" && if [ -f user-server ]; then mv -f user-server user-server.prev; fi && mv -f user-server.new user-server"
  log "  二进制已生成：$ROOT/user-server/bin/user-server（上一版在 .prev，回滚= mv user-server.prev user-server）"

  # 2) 停止旧的 user-server 进程
  # 兼容三种历史启动方式：../user-server/bin/user-server、./bin/user-server、
  # 旧版 /tmp/hivemtk-user-server。模式首字符加 [] 是防止匹配到包裹 pkill 的
  # bash -c 命令行本身（自匹配会把部署脚本误杀）；末尾再按端口兜底清理残留监听。
  log "==> 停止旧 user-server 进程"
  # 只匹配部署产物 user-server/bin/user-server（含 [u] 前缀防自匹配）。
  # 不再匹配 [h]ivemtk-user-server / [b]in/user-server：后者会命中含 "bin/user-server"
  # 字样的无关路径（如 air dev 的 tmp/main、其他项目的 bin 目录），误杀风险高。
  run "pkill -f 'user-server/bin/[u]ser-server' 2>/dev/null || true"
  run "lsof -ti tcp:${USER_SERVER_PORT} 2>/dev/null | xargs kill 2>/dev/null || true"
  run "sleep 1"

  # 3) 启动新 user-server 进程（systemd 优先，回落 nohup；都加载本地 .env）
  # nohup 回落段的 cwd 必须是 user-server/ —— config.yaml 用相对路径 os.ReadFile("config.yaml")，
  # 否则读不到配置文件会回落到 docker 服务名 postgres-user:8202 导致 DB 连接失败。
  run "mkdir -p \"$ROOT/user-server/logs\""
  # 日志轮转：nohup 形态下 logs/user-server.log 没有任何轮转机制，只会一直涨
  # （实测曾积 312MB）。启动前超 200MB 就改名留一份（旧轮转件先清掉），
  # 避免新进程继续往巨型文件里追加。systemd 形态日志走 journald，此步无害。
  log "==> 日志轮转检查（logs/user-server.log，>200MB 改名留档）"
  run "cd \"$ROOT/user-server\" && if [ -f logs/user-server.log ] && [ \"\$(wc -c < logs/user-server.log)\" -gt 209715200 ]; then rm -f logs/user-server.log.*; mv logs/user-server.log logs/user-server.log.\$(date +%Y%m%d_%H%M%S); fi"
  # 监听端口只认 env PORT（cmd/api/main.go resolveListenAddr），USER_SERVER_PORT 只用来拼探测地址，
  # 两者历史上要靠人工同步改。这里在 source .env 之后补一次回落：.env 给了 PORT 就用它的，
  # 没给才用 USER_SERVER_PORT —— 否则 .env 只设 USER_SERVER_PORT 时服务听 8204、探测打去别处，
  # 健康检查必然误报失败。转义后的 ${PORT:-…} 由内层 bash 求值（此时 .env 已加载）。
  if command -v systemctl >/dev/null 2>&1 && systemctl is-enabled hivemtk-user.service >/dev/null 2>&1; then
    # systemd 守护已启用（unit 模板：deploy/systemd/hivemtk-user.service）：
    # 重启 unit，日志走 stdout → journald（journalctl -u hivemtk-user.service -f），
    # 跳过 nohup 段。unit 内 source 的是部署根 .env，与 nohup 路径同语义。
    log "==> 启动 user-server（systemd unit hivemtk-user.service，端口 ${USER_SERVER_PORT}）"
    run "systemctl restart hivemtk-user.service"
  else
    # 回落：本机没有启用对应 unit 时保持原 nohup 启动（向后兼容，语义不变）
    log "==> 启动 user-server（nohup，端口 ${USER_SERVER_PORT}）"
    run "cd \"$ROOT\" && set -a && [ -f .env ] && source .env; set +a && export PORT=\"\${PORT:-${USER_SERVER_PORT}}\" && cd user-server && nohup ../user-server/bin/user-server > logs/user-server.log 2>&1 &"
  fi
  run "sleep 2"

  # 4) 本地健康检查
  log "==> 本地健康检查 ${HEALTH_URL}"
  local i=0
  if [[ -n "$DRY_RUN" ]]; then
    # dry-run 不真启动进程，无健康可探（否则会对着没起的端口硬等 30s 再 die）
    log "  [dry-run] 跳过本地健康检查"
  elif [[ -n "$SKIP_HEALTHCHECK" ]]; then
    # 逃生门：与 healthcheck() 同一语义，SKIP_HEALTHCHECK=1 时跳过（含硬失败分支）
    log "  跳过本地健康检查（SKIP_HEALTHCHECK=1）"
    write_deploy_version
  else
    while (( i < 30 )); do
      # 探针与 body 判定见 probe_ok()：/health + grep "status":"ok" 且无 degraded。
      # 注意：变量后紧跟全角括号‘）’ 时，Bash 会把 i） 当成变量名，
      # 在 set -u 下报 unbound variable；务必用 ${i} 显式定界。
      if probe_ok "$HEALTH_URL"; then
        log "  ✅ user-server 已启动（${HEALTH_URL}，${i}s）"
        write_deploy_version
        return 0
      fi
      sleep 1
      i=$((i+1))
    done
    # 健康检查是"发布是否成功"的唯一自证：超时必须硬失败，不能 log_warn 后照常报"发布完成"
    die "user-server 本地健康检查失败（30s 内 ${HEALTH_URL} 未通过），请查看 user-server/logs/user-server.log；确认无碍需跳过时设 SKIP_HEALTHCHECK=1"
  fi

  log "  user-server 部署完成"
}

# ---------- 更新静态托管配置（反向代理层同源托管）----------
# 2026-09-03 注：函数体暂缺，--反向代理层-only 模式不生效。
# 静态资源已通过 push_web() rsync 到云端 /www/wwwroot/hivemtk/user-web/dist/
deploy_reverse_proxy() {
  die "反向代理部署未实现：请按 docs/operations/reverse-proxy/*.template 手动配置"
}

healthcheck() {
  [[ -n "$DRY_RUN" ]] && { log "[dry-run] 跳过健康检查（未实际启动进程）"; return 0; }
  [[ -n "$SKIP_HEALTHCHECK" ]] && { log "跳过健康检查"; return 0; }
  log "########## 健康检查 ##########"

  local max_wait=60
  local i=0
  while (( i < max_wait )); do
    # 探针与 body 判定见 probe_ok()：打 /health 并 grep 响应体。
    # 不能只看 HTTP 状态码（degraded 仍回 200），也不能只看 Content-Type
    # （/api/health 空探针恒 200+JSON，反向代理层 SPA 兜底也会回 200+HTML）。
    if probe_ok "$HEALTH_URL"; then
      # 注意：变量后紧跟全角括号‘）’ 时，Bash 会把 i） 当成变量名，
      # 在 set -u 下报 unbound variable；务必用 ${i} 显式定界。
      log "  ✅ ${HEALTH_URL} 通过（${i}s）"
      return 0
    fi
    sleep 3
    i=$((i+3))
  done

  # 硬失败：健康检查不过＝发布没完成，脚本必须非零退出，不许再打"✅ 发布完成"
  die "健康检查失败（${max_wait}s 内 ${HEALTH_URL} 未通过），请手动检查；确认无碍需跳过时设 SKIP_HEALTHCHECK=1"
}

# ===================== 主流程 =====================
preflight

# 跨仓漂移检查：平台链路开着（PLATFORM_ENABLED=true）时在发布开头核对两端
# 协议版本常量 / MERCHANT_API_SECRET / platform.yaml 占位符。FAIL 拦发布，
# WARN 仅提示（详见 scripts/check-cross-repo-drift.sh 头注）。
if platform_enabled; then
  bash "$ROOT/scripts/check-cross-repo-drift.sh" || die "跨仓漂移检查 FAIL（红因见上方 [drift] 行），已阻止发布"
fi

case "$MODE" in
  web)
    build_web
    push_web
    ;;
  api)
    deploy_api
    ;;
  反向代理层)
    deploy_reverse_proxy
    ;;
  all)
    # 先 API 后前端：API 编译/发布/健康检查全过才推前端，
    # 避免 API 编译失败但前端已推出去造成的版本错配（前端按旧 API 契约渲染）。
    deploy_api
    build_web
    push_web
    ;;
  *) die "未知模式: $MODE" ;;
esac

# web 模式只推静态资源，本次没有（重新）部署 user-server：本地 8204 探的是
# 另一套进程（生产形态下 API 在远端容器里），拿它自证本次发布只会误报失败。
if [[ "$MODE" == "web" ]]; then
  log "  本次仅发布前端，跳过 user-server 健康检查"
else
  healthcheck
fi

if [[ -n "$DRY_RUN" ]]; then
  log "发布完成（--dry-run 仅预览，未实际执行）。"
else
  log "=========================================="
  log "✅ 发布完成！"
  log "  本地 API: ${HEALTH_URL%/health}"
  log "  二进制回滚点: user-server/bin/user-server.prev（回滚= mv user-server.prev user-server 后重启）"
  if mode_needs_remote; then
    log "  前端已推送到: $DEPLOY_USER@$DEPLOY_HOST:$WEBROOT_USERWEB_DIST/"
  else
    log "  本次无远端推送（纯本地模式）"
  fi
  log "=========================================="
fi

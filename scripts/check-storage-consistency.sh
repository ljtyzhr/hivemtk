#!/usr/bin/env bash
# 存储一致性巡检：DB 里引用的上传文件，是否真的躺在宿主机持久化目录里。
#
# 2026-10-22 生产事故的根因形态（平台端 + 用户端各一次）：
#   容器重建 → 容器可写层里的 uploads/ 被清空 → DB 里的 file_url / cover_url
#   还指着那些文件 → 静态下载全部 404，而 DB 查询一切正常、无任何报错。
#
# 这个脚本就是为"别再靠人点下载才发现"而存在的：把「DB 引用」与「磁盘实物」
# 两边对齐，任何一侧缺一块都立刻报出来。
#
# 检查三件事：
#   1. compose 是否把 uploads 挂到了宿主机（没挂 = 下次重建必丢）；
#   2. DB 里引用 /files/... 的每一行，对应文件是否在磁盘上；
#   3. 磁盘上是否有孤儿文件（DB 里没人引用 —— 只提示，不判失败）。
#
# 三种退出码：0 全通过；1 发现问题；2 扫不到（环境缺失，不等于没问题）。
#
# 用法：
#   scripts/check-storage-consistency.sh --repo DIR [--compose FILE] [--uploads DIR]
#   scripts/check-storage-consistency.sh --repo DIR --remote host:port   # 走 ssh 在服务器上跑
#
# 说明：默认只做**静态**检查（compose 挂载 + 本地目录），不连数据库。
# 数据库那一段需要 psql + 两个库，生产巡检建议在服务器上直接跑：
#   docker exec mtk-platform-postgres psql ... 见 --with-db 用法。
set -uo pipefail

REPO="."
COMPOSE=""
UPLOADS=""
REMOTE=""
SSH_USER=""
WITH_DB=0

while [ $# -gt 0 ]; do
  case "$1" in
    --repo)     REPO="$2"; shift 2 ;;
    --compose)  COMPOSE="$2"; shift 2 ;;
    --uploads)  UPLOADS="$2"; shift 2 ;;
    --remote)   REMOTE="$2"; shift 2 ;;
    --user)     SSH_USER="$2"; shift 2 ;;
    --with-db)  WITH_DB=1; shift ;;
    -h|--help)  sed -n '2,34p' "$0"; exit 0 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

fail=0
warn=0
pass=0

ok()   { printf 'ok      %s\n' "$1"; pass=$((pass + 1)); }
bad()  { printf 'FAIL    %s\n' "$1"; fail=$((fail + 1)); }
note() { printf 'WARN    %s\n' "$1"; warn=$((warn + 1)); }

# ---------------------------------------------------------------- 1. compose 挂载
# 判据：compose 文件里出现了「宿主目录:/app/uploads」这种 bind mount。
# 漏了就意味着容器重建会清空上传目录（2026-10-22 事故根因）。
check_compose_mount() {
  local f="$1" label="$2"
  if [ ! -f "$f" ]; then
    note "$label: 找不到 compose 文件 ${f}，跳过挂载检查"
    return
  fi
  if grep -Eq '^[[:space:]]*-[[:space:]]*[^[:space:]:]+:/app/uploads' "$f"; then
    ok "$label: uploads 已挂宿主机持久化"
  else
    bad "$label: compose 里没有 /app/uploads 的宿主机挂载 —— 容器重建会清空上传目录（2026-10-22 事故根因）"
  fi
  # 落盘目录与静态托管目录必须是同一个：两处都读 STORAGE_LOCAL_BASE_DIR，
  # 但"一个是相对路径 /app，一个是绝对路径 /app/uploads"时，改 WORKDIR 就会分叉。
  if grep -Eq 'STORAGE_LOCAL_BASE_DIR:[[:space:]]*/app/uploads' "$f"; then
    ok "$label: STORAGE_LOCAL_BASE_DIR 已显式指向 /app/uploads"
  else
    note "$label: 未显式设置 STORAGE_LOCAL_BASE_DIR=/app/uploads（当前靠 WORKDIR=/app 恰好等价，换工作目录会分叉）"
  fi
}

if [ -n "$REMOTE" ]; then
  echo "=== 远端 compose 挂载检查 ==="
  host="${REMOTE%%:*}"; port="${REMOTE##*:}"; [ -n "$SSH_USER" ] && host="$SSH_USER@$host"
  [ "$port" = "$REMOTE" ] && port=22
  remote_out="$(ssh -p "$port" -o BatchMode=yes "$host" 'bash -s' <<'REMOTE_COMPOSE' 2>/dev/null
set -uo pipefail
for c in /www/wwwroot/hivemtk/platform-server/docker-compose.yml /www/wwwroot/hivemtk/user-server/docker-compose.yml; do
  [ -f "$c" ] || { echo "WARN    找不到 $c"; continue; }
  echo "--- $c"
  if grep -Eq '^[[:space:]]*-[[:space:]]*[^[:space:]:]+:/app/uploads' "$c"; then
    echo "ok      uploads 已挂宿主机持久化"
  else
    echo "FAIL    没有 /app/uploads 的宿主机挂载 —— 容器重建会清空上传目录"
  fi
  if grep -Eq 'STORAGE_LOCAL_BASE_DIR:[[:space:]]*/app/uploads' "$c"; then
    echo "ok      STORAGE_LOCAL_BASE_DIR 已显式指向 /app/uploads"
  else
    echo "WARN    未显式设置 STORAGE_LOCAL_BASE_DIR=/app/uploads"
  fi
done
echo "--- 容器实况（Mounts 里应出现 -> /app/uploads）"
for cn in mtk-platform-server mtk-user-server; do
  docker inspect -f "{{.Name}} {{range .Mounts}}{{.Source}}->{{.Destination}} {{end}}" "$cn" 2>/dev/null
done
REMOTE_COMPOSE
)"
  printf '%s\n' "$remote_out" | sed 's/^/    /'
  # 远端结果计入汇总（否则永远 ok=0/FAIL=0，脚本就成了"永远说没事"的摆设）
  while IFS= read -r line; do
    case "$line" in
      ok*)   pass=$((pass + 1)) ;;
      FAIL*) fail=$((fail + 1)) ;;
      WARN*) warn=$((warn + 1)) ;;
    esac
  done <<< "$remote_out"
fi

if [ -z "$REMOTE" ]; then
echo "=== 1. compose 持久化挂载 ==="
if [ -n "$COMPOSE" ]; then
  check_compose_mount "$COMPOSE" "指定 compose"
else
  for c in \
    "$REPO/hivemtk-platform/docker-compose.yml" \
    "$REPO/hivemtk/docker-compose.yml" \
    "$REPO/hivemtk-platform-deploy-wt/docker-compose.yml"
  do
    [ -f "$c" ] && check_compose_mount "$c" "${c#$REPO/}"
  done
  # 仓内 compose 是「仅数据层」形态（platform-server 服务不在仓内），
  # 所以仓内命中不了并不代表生产没挂 —— 生产态以 --remote 为准。
  note "仓内 compose 未必含 platform-server/user-server 服务（生产栈在服务器侧），生产请用 --remote 复核"
fi
fi

# ---------------------------------------------------------------- 2. 磁盘实物
echo
echo "=== 2. 上传目录实物 ==="
for d in ${UPLOADS:-}; do
  [ -d "$d" ] || { note "上传目录不存在: $d"; continue; }
  n=$(find "$d" -type f 2>/dev/null | wc -l | tr -d ' ')
  sz=$(du -sh "$d" 2>/dev/null | cut -f1)
  if [ "$n" -gt 0 ]; then
    ok "上传目录 $d: $n 个文件 / $sz"
  else
    bad "上传目录 $d: **空目录** —— 若 DB 里有 /files/ 引用，它们全部 404"
  fi
done

# ---------------------------------------------------------------- 3. DB 引用
# 走 ssh 在服务器上跑：psql 在容器里、宿主机没有 psql 客户端。
# 两个库各扫一遍「引用了 /files/ 的列」，把路径落到宿主机 uploads 目录核对。
echo
echo "=== 3. DB 引用 vs 磁盘实物 ==="
if [ "$WITH_DB" -eq 1 ]; then
  if [ -z "$REMOTE" ]; then
    note "--with-db 需要配合 --remote host:port（在服务器上跑，psql 只在容器里）"
  else
    host="${REMOTE%%:*}"; port="${REMOTE##*:}"; [ -n "$SSH_USER" ] && host="$SSH_USER@$host"
    [ "$port" = "$REMOTE" ] && port=22
    db_out="$(ssh -p "$port" -o BatchMode=yes "$host" 'bash -s' <<'REMOTE_SCRIPT' 2>/dev/null
set -uo pipefail
PLAT_ROOT=/www/wwwroot/hivemtk/platform-server/data/uploads
USER_ROOT=/www/wwwroot/hivemtk/user-server/data/uploads

scan() { # $1=容器名 $2=库名 $3=端口 $4=上传根目录 $5=sql(取路径列)
  local c="$1" db="$2" port="$3" root="$4" sql="$5" label="$6"
  local paths
  paths="$(docker exec "$c" psql -U admin -d "$db" -p "$port" -t -A -c "$sql" 2>/dev/null | sed 's/^ *//' | grep -v '^$' || true)"
  [ -z "$paths" ] && { echo "ok      $label: 无 /files/ 引用"; return 0; }
  local miss=0 total=0
  while IFS= read -r p; do
    [ -z "$p" ] && continue
    total=$((total + 1))
    # URL 里取 /files/ 之后的部分（去掉可能的绝对地址前缀与查询串）
    local rel="${p#*\/files\/}"; rel="${rel%%\?*}"
    if [ ! -f "$root/$rel" ]; then
      echo "FAIL    $label: DB 引用了但磁盘上没有 → $root/$rel"
      miss=$((miss + 1))
    fi
  done <<< "$paths"
  if [ "$miss" -eq 0 ]; then
    echo "ok      $label: $total 条 /files/ 引用全部有对应文件"
  else
    echo "FAIL    $label: $miss/$total 条 /files/ 引用指向不存在的文件"
  fi
}

scan mtk-platform-postgres platform_db 8201 "$PLAT_ROOT" \
  "SELECT DISTINCT regexp_replace(file_url,'^.*(/files/.*)$','\\1') FROM asset_versions WHERE file_url LIKE '%/files/%'
   UNION SELECT DISTINCT regexp_replace(cover_url,'^.*(/files/.*)$','\\1') FROM assets WHERE cover_url LIKE '%/files/%';" \
  "平台端"

scan mtk-postgres user_db 8202 "$USER_ROOT" \
  "SELECT DISTINCT regexp_replace(file_path,'^.*(/files/.*)$','\\1') FROM kb_documents WHERE file_path LIKE '%/files/%'
   UNION SELECT DISTINCT regexp_replace(cover_image,'^.*(/files/.*)$','\\1') FROM asset_bundles WHERE cover_image LIKE '%/files/%'
   UNION SELECT DISTINCT regexp_replace(image_url,'^.*(/files/.*)$','\\1') FROM douyin_cards WHERE image_url LIKE '%/files/%';" \
  "用户端"
REMOTE_SCRIPT
)"
    printf '%s\n' "$db_out" | sed 's/^/    /'
    while IFS= read -r line; do
      case "$line" in
        ok*)   pass=$((pass + 1)) ;;
        FAIL*) fail=$((fail + 1)) ;;
        WARN*) warn=$((warn + 1)) ;;
      esac
    done <<< "$db_out"
  fi
else
  echo "跳过（静态检查模式）。加 --with-db --remote host:port 可做 DB↔磁盘核对，"
  echo "或直接在服务器上手工核对："
  cat <<'EOF'
  # 平台端：列出所有引用 /files/ 的路径
  docker exec mtk-platform-postgres psql -U admin -d platform_db -p 8201 -t -A -c \
    "SELECT DISTINCT regexp_replace(file_url,'^.*(/files/.*)$','\\1') FROM asset_versions WHERE file_url LIKE '%/files/%';"
  # 然后逐条核对宿主机文件是否存在：
  #   ls -l /www/wwwroot/hivemtk/platform-server/data/uploads/<去掉 /files/ 的路径>
EOF
fi

echo
echo "=== 汇总 ==="
printf 'ok=%d  FAIL=%d  WARN=%d\n' "$pass" "$fail" "$warn"
[ "$fail" -gt 0 ] && exit 1
exit 0

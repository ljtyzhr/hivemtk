#!/usr/bin/env bash
# pre-push-secrets.sh —— 把两道"只能在本机跑"的密钥闸挂到推送前
#
# 为什么必须有执行点（E-11，2026-10-23 现测）：
#   check-secrets.sh 与 check-secret-containment.py 自登记以来**只有人手动跑**这一种入口：
#   .githooks/ 里只有 commit-msg，.github/workflows/ 对这两个文件零引用，`make audit` 也不含
#   audit-secrets。手动跑的代价是"红是不是常态"没人说得清 —— 恒红的门与坏掉的门长得一样。
#
# 为什么这两道只能在本机跑、不上 CI：
#   A 段与包含闸的候选源都是本机 `.env` ＋活容器 env。把真凭证推上 runner 等于第二次泄露，
#   而 CI 上取不到同一批值 ⇒ 注册进 CI 只会得到一扇恒退 2 的哑门。CI 侧这一维由
#   .github/workflows/secret-patterns.yml 跑 `check-secrets.sh --patterns-only`（B 段）＋ gitleaks。
#
# 退出码（与本仓"2 是门没跑过、不是绿也不是泄露"的口径一致，三档都不许合并）：
#   0 = 两道闸都跑过且干净，放行推送；
#   1 = 发现了明文凭证，拦下；
#   2 = 有一道闸没能执行（缺 .env、仓不在原位、不在 git 仓内），同样拦下 —— 宁可拦下一次
#       误伤，也不放行一次"其实什么都没比"的推送。
#
# 这里**不**走 `make audit-secrets`：make 在配方失败时统一退 2，会把"发现明文"（脚本退 1）
# 与"没跑过"（脚本退 2）压成同一个数 —— 正是上面要分开的那两档。配方本身就是一行
# `bash scripts/check-secrets.sh`，直接调它，读到的就是脚本自己的码。
set -uo pipefail

ROOT=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "❌ pre-push：拿不到仓根，密钥闸无法执行" >&2
  exit 2
}
cd "$ROOT" || exit 2

REMOTE=${1:-<未知远端>}
URL=${2:-<未知地址>}

echo "══════ pre-push 密钥闸（远端 ${REMOTE}）══════"

run_gate() { # $1=人话名字 $2…=命令
  local label=$1
  shift
  echo "── ${label} ──"
  "$@" </dev/null
  local rc=$?
  case $rc in
    0) echo "→ ${label}：通过" ;;
    1) echo "→ ${label}：❌ 发现明文凭证" ;;
    *) echo "→ ${label}：⚠️ 未能执行（rc=${rc}），本闸这一维没有读数" ;;
  esac
  return $rc
}

FINAL=0
run_gate "A+B 明文凭证门（check-secrets.sh）" bash scripts/check-secrets.sh
rc_ab=$?
run_gate "真值包含闸（check-secret-containment.py）" python3 scripts/check-secret-containment.py
rc_ct=$?

if [[ $rc_ab == 1 || $rc_ct == 1 ]]; then
  FINAL=1
elif [[ $rc_ab != 0 || $rc_ct != 0 ]]; then
  FINAL=2
fi

if [[ $FINAL != 0 ]]; then
  cat >&2 <<MSG
❌ 推送被 pre-push 密钥闸拦下（rc=${FINAL}）。
   1＝待纳管文件里出现了明文凭证：把它换成环境变量注入，或确属公开常量时
     在 scripts/.secret-allowlist 里逐值登记并写明理由（整类放行不算）。
   2＝闸门没跑起来：本仓要求仓根有 .env（或用 ENV_FILE=／HUB_ENV_FILE= 指过去），
     并在仓根执行；判据文件 docs/architecture/CHANNEL_INTEGRATION_AUDIT_2026-09.md
     与 scripts/cap-ab-53300-probe.py 必须都在原位。
   这道闸刻意只在本机跑（候选源是本机 .env 与活容器，CI 拿不到真凭证也不该拿到）。
   确需绕过（例如临时推一个不含凭证的分支而本机环境未就绪）：git push --no-verify，
   但请把绕开的原因写进 PR／提交说明，别让它变成习惯。
MSG
  exit $FINAL
fi

echo "✅ pre-push 密钥闸：两道都跑过且干净，放行推送"

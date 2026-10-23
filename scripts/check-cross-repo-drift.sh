#!/usr/bin/env bash
# =============================================================================
# check-cross-repo-drift.sh —— hivemtk（用户端）× hivemtk-platform（平台端）跨仓漂移检查
#
# 两仓通过 merchant-api HMAC + 协议版本常量耦合，任何一端单独升级都可能静默断链。
# 本脚本在发布前核对三处：
#   ① 协议版本常量一致（不一致 → WARN：需按协议同步升级，不强拦）
#   ② MERCHANT_API_SECRET 两端同值（不等 → FAIL：HMAC 两端必须同值，签名必然对不上）
#   ③ user-server/config/platform.yaml 的 secret 必须是 ${MERCHANT_API_SECRET} 占位符
#      （被改成字面值 → WARN：环境变量轮换会静默失效）
#
# 用法:  bash scripts/check-cross-repo-drift.sh
# 退出码: 0 = 无 FAIL（可有 WARN）；1 = 存在 FAIL
# 调用点: 手工发布流程在 PLATFORM_ENABLED=true 时于发布开头调用，FAIL 即停。
# =============================================================================
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLATFORM_ROOT="$(cd "$ROOT/../hivemtk-platform" 2>/dev/null && pwd || echo "")"

# ---------- 平台仓不在同级 → 跳过 ----------
if [[ -z "$PLATFORM_ROOT" || ! -d "$PLATFORM_ROOT" ]]; then
  echo "[drift] 平台仓不在同级（$ROOT/../hivemtk-platform 不存在），跳过"
  exit 0
fi

PASS_N=0; WARN_N=0; FAIL_N=0
pass() { echo "[drift] PASS: $*"; PASS_N=$((PASS_N+1)); }
warn() { echo "[drift] WARN: $*"; WARN_N=$((WARN_N+1)); }
fail() { echo "[drift] FAIL: $*"; FAIL_N=$((FAIL_N+1)); }

# 从 .env 里抽一个键的值（取最后一处赋行；去首尾空白与成对引号；忽略注释行）
env_val() { # $1=.env路径 $2=键
  local raw
  raw="$(grep -E "^${2}=" "$1" 2>/dev/null | tail -1 || true)"
  raw="${raw#*=}"
  # 去首尾空白
  raw="$(printf '%s' "$raw" | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//')"
  # 去成对引号
  case "$raw" in
    \"*\") raw="${raw#\"}"; raw="${raw%\"}" ;;
    \'*\') raw="${raw#\'}"; raw="${raw%\'}" ;;
  esac
  printf '%s' "$raw"
}

# ---------- 检查①：协议版本常量一致 ----------
R1_VER_FILE="$ROOT/user-server/internal/service/tenant.go"
R2_VER_FILE="$PLATFORM_ROOT/platform-server/internal/service/tenant_registration_service.go"
r1_ver=""; r2_ver=""
[[ -f "$R1_VER_FILE" ]] && r1_ver="$(sed -nE 's/.*serviceCurrentAgreementVersion[[:space:]]*=[[:space:]]*"([^"]*)".*/\1/p' "$R1_VER_FILE" | head -1)"
[[ -f "$R2_VER_FILE" ]] && r2_ver="$(sed -nE 's/.*const CurrentAgreementVersion[[:space:]]*=[[:space:]]*"([^"]*)".*/\1/p' "$R2_VER_FILE" | head -1)"

if [[ -z "$r1_ver" || -z "$r2_ver" ]]; then
  warn "协议版本常量抽不到（R1='${r1_ver:-<无>}' R2='${r2_ver:-<无>}'；文件或常量改名了就同步本脚本的提取式）"
elif [[ "$r1_ver" != "$r2_ver" ]]; then
  warn "协议版本常量不一致：R1 serviceCurrentAgreementVersion=${r1_ver} vs R2 CurrentAgreementVersion=${r2_ver}（两端需按协议同步升级）"
else
  pass "协议版本常量一致：$r1_ver"
fi

# ---------- 检查②：MERCHANT_API_SECRET 两端同值（HMAC 链路的根） ----------
R1_ENV="$ROOT/.env"
R2_ENV="$PLATFORM_ROOT/.env"
if [[ -f "$R1_ENV" && -f "$R2_ENV" ]]; then
  s1="$(env_val "$R1_ENV" "MERCHANT_API_SECRET")"
  s2="$(env_val "$R2_ENV" "MERCHANT_API_SECRET")"
  if [[ -n "$s1" && -n "$s2" ]]; then
    if [[ "$s1" == "$s2" ]]; then
      pass "MERCHANT_API_SECRET 两端同值（长度 ${#s1}）"
    else
      fail "MERCHANT_API_SECRET 两端不一致（R1 长度 ${#s1} vs R2 长度 ${#s2}）——HMAC 签名两端必须同值，merchant-api 必然 401；请对齐两端 .env 后重发"
    fi
  else
    echo "[drift] 跳过检查②：一端 .env 缺 MERCHANT_API_SECRET（R1=${s1:+有}${s1:-无} R2=${s2:+有}${s2:-无}）"
  fi
else
  echo "[drift] 跳过检查②：.env 不齐（R1=$([[ -f $R1_ENV ]] && echo 有 || echo 无) R2=$([[ -f $R2_ENV ]] && echo 有 || echo 无)）"
fi

# ---------- 检查③：platform.yaml 的 secret 必须走 ${MERCHANT_API_SECRET} 占位符 ----------
R1_YAML="$ROOT/user-server/config/platform.yaml"
if [[ -f "$R1_YAML" ]]; then
  if grep -Eq '^[[:space:]]*secret:[[:space:]]*"?\$\{MERCHANT_API_SECRET\}"?[[:space:]]*$' "$R1_YAML"; then
    pass "platform.yaml secret 走 \${MERCHANT_API_SECRET} 占位符"
  else
    warn "platform.yaml 的 secret 不是 \${MERCHANT_API_SECRET} 占位符（${R1_YAML}）——改成字面值会让密钥轮换静默失效，请改回占位符"
  fi
else
  warn "未找到 ${R1_YAML}（文件改名/移位了就同步本脚本）"
fi

# ---------- 汇总 ----------
echo "[drift] 汇总：PASS=${PASS_N} WARN=${WARN_N} FAIL=${FAIL_N}"
[[ "$FAIL_N" -eq 0 ]] || exit 1
exit 0

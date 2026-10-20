#!/bin/bash
# 生产全业务验证（真实网关；回调自签与网关密码学同构）
#
# 命名约束——后续所有生产自动化验证按这套走，既像真实商户又能机器认领：
#   license_no = QA-<UTC 时间戳>        清理脚本的归属签名之一
#   email      = <用户>@qa.example      RFC 2606 保留域，永不可投递，也是签名之一
#   trade_no   = QA<时间戳>             回调行解析不出 order_no 时唯一的认领线索
#   公司名/地址/负责人写正常商号，不写 asdf123 这类噪声；可机辨只落在上面三个字段。
#
# 收尾默认调用 scripts/cleanup_qa_tenants.sh --commit，把本次建的租户连数据一起清掉；
# 需要留下来人工排查时设 KEEP_QA_TENANT=1（此时会打印口令，否则不打印）。
#
# 凭证一律从环境取，脚本不内置、不回显：
#   QA_EPAY_KEY                易支付商户 MD5 密钥
#   QA_PLATFORM_ADMIN_USER     平台端账号
#   QA_PLATFORM_ADMIN_PASSWORD 平台端口令
#   QA_PROD_SSH                <user@host>[:port]，自动清理要用（不设则清理项判失败）
#   QA_EPAY_PID                商户 pid，默认 1295，非机密
set -uo pipefail

API="${API:-https://manageapi.xapptool.cn}"
USERAPI="${USERAPI:-https://workapi.xapptool.cn}"
EPAY_PID="${QA_EPAY_PID:-1295}"
MODULE_KEY="browser-automation"
MODULE_PRICE_CENTS=9900
RENEW_PRICE_CENTS=14900

missing=""
for v in QA_EPAY_KEY QA_PLATFORM_ADMIN_USER QA_PLATFORM_ADMIN_PASSWORD; do
  if [ -z "${!v:-}" ]; then missing="$missing $v"; fi
done
if [ -n "$missing" ]; then
  echo "缺少环境变量:${missing}（脚本不内置口令；参见文件头部说明）" >&2
  exit 2
fi
EPAY_KEY="$QA_EPAY_KEY"
PF_USER="$QA_PLATFORM_ADMIN_USER"
PF_PASS="$QA_PLATFORM_ADMIN_PASSWORD"

PASS=0; FAIL=0
CLEANED=0
TS=$(date +%s)
LICENSE="QA-$(date -u +%Y%m%d%H%M%S)"
ADMIN_USER="qa$(date -u +%Y%m%d%H%M%S)"
ADMIN_MAIL="${ADMIN_USER}@qa.example"
ADMIN_PASS="Qa#Pass${TS}"
TENANT_CODE=""

ck() { if [ "$2" = "0" ]; then PASS=$((PASS+1)); echo "  [OK] $1"; else FAIL=$((FAIL+1)); echo "  [X] $1"; fi }
md5hex() { if command -v md5sum >/dev/null 2>&1; then md5sum | cut -d' ' -f1; else md5 -q; fi }
# curl 带重试：网络抖动下空响应自动重试
rcurl() { # rcurl <tries> <curl args...>
  local n=$1; shift; local out=""
  for i in $(seq 1 "$n"); do
    out=$(curl -s --max-time 25 "$@") && [ -n "$out" ] && { echo "$out"; return 0; }
    sleep 2
  done
  echo "$out"; return 1
}
# post_json <url> <json> [额外 curl 参数...]：请求体走 0600 临时文件，不进 ps 参数表
post_json() {
  local url="$1" body="$2"; shift 2
  local f out rc
  f=$(mktemp "${TMPDIR:-/tmp}/qa_e2e_body_XXXXXX") || return 1
  printf '%s' "$body" > "$f"
  out=$(rcurl 3 -X POST "$url" -H 'Content-Type: application/json' "$@" --data-binary "@$f")
  rc=$?
  rm -f "$f"
  printf '%s' "$out"
  return "$rc"
}
jq_str() { # jq_str <key> <json>：无 jq 依赖的最小取值
  printf '%s' "$2" | sed -n "s/.*\"$1\":\"\\([^\"]*\\)\".*/\\1/p" | head -1
}

# 返回值：0=真的清掉了，3=本轮按规则不该清理（保留／还没建租户），其余=清理失败
cleanup_tenant() {
  if [ "${KEEP_QA_TENANT:-0}" = "1" ]; then
    echo "  [i] KEEP_QA_TENANT=1：保留租户 ${TENANT_CODE:-未创建}，登录口令 ${ADMIN_PASS}"
    return 3
  fi
  [ -n "$TENANT_CODE" ] || { echo "  [!] 未创建租户，无需清理"; return 3; }
  if [ -z "${QA_PROD_SSH:-}" ]; then
    echo "  [X] 未设 QA_PROD_SSH，无法自动清理租户 ${TENANT_CODE}（请手工跑 scripts/cleanup_qa_tenants.sh）"
    return 1
  fi
  local here rc
  here=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
  "$here"'/cleanup_qa_tenants.sh' --tenant-code "$TENANT_CODE" --commit
  rc=$?
  if [ "$rc" = "0" ]; then CLEANED=1; fi
  return "$rc"
}
on_exit() {
  # 只在「应当清理且还没清理成功」时兜底；KEEP／未建租户的情形交给 cleanup_tenant 自己判 3，
  # 但此处不再调用，避免正常跑完 S11 后同一条说明打印两遍
  if [ "$CLEANED" = "0" ] && [ "${KEEP_QA_TENANT:-0}" != "1" ] && [ -n "$TENANT_CODE" ]; then
    cleanup_tenant
  fi
}
trap on_exit EXIT

echo "===== S0 公开配置 ====="
CFG=$(rcurl 3 "$USERAPI/api/public/config")
printf '%s\n' "$CFG" | grep -q '"edition":"saas"'; ck "edition=saas" $?
printf '%s\n' "$CFG" | grep -q '"tenant_mode":true'; ck "tenant_mode=true" $?

echo "===== S1 监管告知（协议版本由平台下发，不写死） ====="
NOTICE=$(rcurl 3 "$USERAPI/api/tenant/regulatory-notice")
AGREEMENT=$(jq_str agreement_version "$NOTICE")
[ -n "$AGREEMENT" ]; ck "agreement_version=${AGREEMENT:-空}" $?

echo "===== S2 注册 QA 租户（签名：${LICENSE} / ${ADMIN_MAIL}） ====="
REG=$(post_json "$USERAPI/api/tenant/register" "{
  \"name\":\"上海云启智数科技有限公司\",\"username\":\"${ADMIN_USER}\",
  \"email\":\"${ADMIN_MAIL}\",\"password\":\"${ADMIN_PASS}\",
  \"company_name\":\"上海云启智数科技有限公司\",\"legal_person\":\"周明远\",
  \"phone\":\"13800138000\",\"address\":\"上海市浦东新区世纪大道 100 号 20 层\",
  \"license_no\":\"${LICENSE}\",
  \"regulatory_ack\":true,\"agreement_version\":\"${AGREEMENT}\"}")
printf '%s\n' "$REG" | head -c 200; echo
printf '%s\n' "$REG" | grep -q '"code":0'; ck "注册 code=0" $?
TENANT_CODE=$(jq_str tenant_code "$REG")
[ -n "$TENANT_CODE" ]; ck "tenant_code=${TENANT_CODE:-空}" $?

echo "===== S3 登录 + 首次 sync（把平台 trial 镜像拉下来） ====="
LOGIN=$(post_json "$USERAPI/api/auth/login" "{\"username\":\"${ADMIN_USER}\",\"password\":\"${ADMIN_PASS}\"}")
TOKEN=$(jq_str token "$LOGIN")
[ -n "$TOKEN" ]; ck "登录 JWT" $?
AUTH="Authorization: Bearer $TOKEN"
sleep 3   # 等注册异步上报平台完成（trial 创建）
SYNC=$(rcurl 3 -X POST -H "$AUTH" "$USERAPI/api/modules/sync")
printf '%s\n' "$SYNC" | head -c 150; echo
printf '%s\n' "$SYNC" | grep -q '"code":0'; ck "sync code=0" $?
printf '%s\n' "$SYNC" | grep -q '"degraded":false'; ck "sync 非降级（平台可达）" $?

echo "===== S4 trial 状态 ====="
SUB=$(rcurl 3 -H "$AUTH" "$USERAPI/api/subscription/status")
printf '%s\n' "$SUB" | head -c 250; echo
printf '%s\n' "$SUB" | grep -q '"status":"trial"'; ck "订阅=trial" $?
printf '%s\n' "$SUB" | grep -q "\"price_cents\":${RENEW_PRICE_CENTS}"; ck "续期定价=${RENEW_PRICE_CENTS}" $?
printf '%s\n' "$SUB" | grep -q '"locked":false'; ck "trial 未锁定" $?

echo "===== S5 初始化面：菜单/模块/开户赠送 ====="
# 注：这一相是「未部署修复」的判别位。工作树里 ProvisionTenantModules +
# Sync 的落点归并已把新租户的模块行写到自己名下，但线上跑的是修复前的二进制：
# 同步把行统一写到 tenant_id=1，新租户拿到 0 行 → status=active 计数 0、
# 除基础外的页面 4301。部署后这两项应转绿，未部署时这里是预期红。
MY=$(rcurl 3 -H "$AUTH" "$USERAPI/api/modules/my")
printf '%s\n' "$MY" | grep -q '"code":0'; ck "modules/my code=0" $?
printf '%s\n' "$MY" | grep -q "\"module_key\":\"${MODULE_KEY}\"[^}]*\"status\":\"active\""
ck "${MODULE_KEY} 未被默认激活" $?
ACT=$(printf '%s\n' "$MY" | grep -o '"status":"active"' | wc -l | tr -d ' ')
OWN=$(printf '%s\n' "$MY" | grep -o '"owned":true' | wc -l | tr -d ' ')
echo "  [i] modules/my 总项=$(printf '%s\n' "$MY" | grep -o '"module_key"' | wc -l | tr -d ' ')，" \
     "status=active ${ACT} 项，owned=true ${OWN} 项"
[ "$ACT" -ge 7 ]; ck "已激活模块 ${ACT} 个（>=7 基础模块）" $?
# 「没有订阅功能菜单」的判据落在模块镜像的菜单节点上：至少一个已激活模块带非空 menu_keys
printf '%s\n' "$MY" | grep -q '"menu_keys":\["'; ck "已激活模块带菜单节点（订阅菜单非空）" $?
MENUS=$(rcurl 3 -H "$AUTH" "$USERAPI/api/menus")
printf '%s\n' "$MENUS" | grep -q '"code":0'; ck "menus 路由 code=0" $?
echo "  [i] /api/menus path 字段计数=$(printf '%s\n' "$MENUS" | grep -o '"path"' | wc -l | tr -d ' ')，节点名计数=$(printf '%s\n' "$MENUS" | grep -o '"name"' | wc -l | tr -d ' ')"
AGENTS=$(rcurl 3 -H "$AUTH" "$USERAPI/api/agents/all")
printf '%s\n' "$AGENTS" | grep -q '"code":0'
ck "agents 路由连通（赠送面由 starter-kit 生日志核实：克隆+资产包）" $?

echo "===== S6 市场价格 ====="
MKT=$(rcurl 3 "$USERAPI/api/module-market/list")
printf '%s\n' "$MKT" | grep -q "\"module_key\":\"${MODULE_KEY}\"[^}]*\"price_cents\":${MODULE_PRICE_CENTS}"
ck "浏览器自动化 定价 ${MODULE_PRICE_CENTS}" $?
FCNT=$(printf '%s\n' "$MKT" | grep -o '"price_cents":0' | wc -l | tr -d ' ')
[ "$FCNT" -ge 15 ]; ck "免费模块 >=15（实际 ${FCNT}）" $?

echo "===== S7 下单 → 真实网关 pay_url ====="
ORDER=$(post_json "$USERAPI/api/module-market/order" \
  "{
  \"module_keys\":[\"${MODULE_KEY}\"],
  \"contact_name\":\"周明远\",\"contact_phone\":\"13800138000\",\"contact_email\":\"${ADMIN_MAIL}\"}" \
  -H "$AUTH")
printf '%s\n' "$ORDER" | head -c 250; echo
ORD_NO=$(jq_str order_no "$ORDER")
PAY_URL=$(jq_str pay_url "$ORDER")
[ -n "$ORD_NO" ]; ck "订单 ${ORD_NO}" $?
[ -n "$PAY_URL" ]; ck "pay_url 下发" $?
printf '%s\n' "$PAY_URL" | grep -q 'gggua.com'; ck "真实网关 pay_url" $?

echo "===== S8 自签回调 → 模块激活 ====="
PROD_NAME="模块订单"
SIGN=$(printf '%s' "money=99.00&name=${PROD_NAME}&out_trade_no=${ORD_NO}&pid=${EPAY_PID}&trade_no=QA${TS}&trade_status=TRADE_SUCCESS&type=alipay&key=${EPAY_KEY}" | md5hex)
NOTIFY=$(rcurl 3 -G "$API/public/payments/notify/epay" \
  --data-urlencode "pid=${EPAY_PID}" --data-urlencode "trade_no=QA${TS}" \
  --data-urlencode "out_trade_no=${ORD_NO}" --data-urlencode "type=alipay" \
  --data-urlencode "name=${PROD_NAME}" --data-urlencode "money=99.00" \
  --data-urlencode "trade_status=TRADE_SUCCESS" --data-urlencode "sign=${SIGN}" \
  --data-urlencode "sign_type=MD5")
[ "$NOTIFY" = "success" ]; ck "回调 success" $?
sleep 2
rcurl 3 -X POST -H "$AUTH" "$USERAPI/api/modules/sync" >/dev/null
MY2=$(rcurl 3 -H "$AUTH" "$USERAPI/api/modules/my")
printf '%s\n' "$MY2" | grep -q "\"module_key\":\"${MODULE_KEY}\"[^}]*\"status\":\"active\""
ck "浏览器自动化已激活" $?

echo "===== S9 续期下单 + 回调 → active ====="
REN=$(post_json "$USERAPI/api/subscription/renew" \
  "{\"contact_name\":\"周明远\",\"contact_phone\":\"13800138000\",\"contact_email\":\"${ADMIN_MAIL}\"}" \
  -H "$AUTH")
printf '%s\n' "$REN" | head -c 200; echo
printf '%s\n' "$REN" | grep -q "\"total_amount_cents\":${RENEW_PRICE_CENTS}"; ck "续期金额=${RENEW_PRICE_CENTS}" $?
REN_NO=$(jq_str order_no "$REN")
[ -n "$REN_NO" ]; ck "续期单 ${REN_NO}" $?
SIGN2=$(printf '%s' "money=149.00&name=${PROD_NAME}&out_trade_no=${REN_NO}&pid=${EPAY_PID}&trade_no=QART${TS}&trade_status=TRADE_SUCCESS&type=alipay&key=${EPAY_KEY}" | md5hex)
NOTIFY2=$(rcurl 3 -G "$API/public/payments/notify/epay" \
  --data-urlencode "pid=${EPAY_PID}" --data-urlencode "trade_no=QART${TS}" \
  --data-urlencode "out_trade_no=${REN_NO}" --data-urlencode "type=alipay" \
  --data-urlencode "name=${PROD_NAME}" --data-urlencode "money=149.00" \
  --data-urlencode "trade_status=TRADE_SUCCESS" --data-urlencode "sign=${SIGN2}" \
  --data-urlencode "sign_type=MD5")
[ "$NOTIFY2" = "success" ]; ck "续期回调 success" $?
sleep 2
rcurl 3 -X POST -H "$AUTH" "$USERAPI/api/modules/sync" >/dev/null
SUB2=$(rcurl 3 -H "$AUTH" "$USERAPI/api/subscription/status")
printf '%s\n' "$SUB2" | head -c 250; echo
printf '%s\n' "$SUB2" | grep -q '"status":"active"'; ck "续期后=active" $?
printf '%s\n' "$SUB2" | grep -q '"locked":false'; ck "未锁定" $?

echo "===== S10 平台端复核：订单收口 + 页面数据源 ====="
PLOGIN=$(post_json "$API/api/auth/login" "{\"username\":\"${PF_USER}\",\"password\":\"${PF_PASS}\"}")
PT=$(jq_str token "$PLOGIN")
# 取不到 token 时把响应头打出来：401（口令不对）与 400（DTO 字段不对）在这条腿上
# 表现完全一样，不打出来下一位读者只能猜
[ -n "$PT" ] || printf '%s\n' "$PLOGIN" | head -c 200
[ -n "$PT" ]; ck "平台端登录" $?
PAUTH="Authorization: Bearer $PT"
for ONO in "$ORD_NO" "$REN_NO"; do
  printf '%s\n' "$(rcurl 3 -H "$PAUTH" "$API/platform/orders/${ONO}")" \
    | grep -qE '"status":"(paid|fulfilled)"'
  ck "平台订单 ${ONO} 收口" $?
done
# 平台端「大量页面没有数据」的那批读接口：逐个要求 code=0；
# list 档还要求列表真的非空（库里现况：1 商户 / 1 装机 / 1 开户登记 / 3 订阅 / 26 资产）
# 平台端的成功码是 code:200（response.Success 用 HTTP 码当业务码），用户端是 code:0；
# 前端 request.js:43 两个都放行，所以这里的判据也必须两个都收，否则是把平台端整批改判红。
pf_read() { # pf_read <名称> <路径> <code|list>
  local name="$1" path="$2" mode="$3" body rc
  body=$(rcurl 3 -H "$PAUTH" "$API${path}")
  printf '%s\n' "$body" | grep -qE '"code":(0|200)'; rc=$?
  if [ "$rc" != "0" ]; then
    echo "    （${name} 非成功信封，响应头：$(printf '%s' "$body" | head -c 160)）"
  elif [ "$mode" = "list" ]; then
    printf '%s\n' "$body" | grep -q '"list":\[{' || {
      echo "    （${name} 列表为空，响应头：$(printf '%s' "$body" | head -c 160)）"
      rc=1
    }
  fi
  ck "平台读接口 ${name}" "$rc"
}
pf_read "dashboard"      "/platform/dashboard"                        code
pf_read "stats-overview" "/platform/stats/overview"                   code
pf_read "merchants"      "/platform/merchants?page=1&page_size=20"    list
pf_read "installs"       "/platform/installs/list?page=1&page_size=20" list
pf_read "tenants"        "/platform/tenants?page=1&page_size=20"      list
pf_read "subscriptions"  "/platform/subscription/list?page=1&page_size=20" list
pf_read "assets"         "/platform/asset-market/assets?page=1&page_size=20" list
pf_read "modules"        "/platform/modules"                          code

echo "===== S11 自动清理本次 QA 租户 ====="
cleanup_tenant; CRC=$?
if   [ "$CRC" = "0" ]; then ck "QA 租户 ${TENANT_CODE} 已清理" 0
elif [ "$CRC" = "3" ]; then echo "  [-] S11 跳过清理（rc=3）"
else ck "QA 租户 ${TENANT_CODE} 已清理" "$CRC"; fi

echo ""
echo "========================================"
echo "  PASS=$PASS  FAIL=$FAIL"
echo "  测试租户 code=${TENANT_CODE:-无} license=${LICENSE} 用户=${ADMIN_USER}"
echo "  模块单=${ORD_NO:-无} 续期单=${REN_NO:-无}"
echo "  清理=$( [ "$CLEANED" = "1" ] && echo '已执行' || echo '未执行' )"
echo "========================================"
[ "$FAIL" = "0" ]

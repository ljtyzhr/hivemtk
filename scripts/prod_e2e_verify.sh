#!/bin/bash
# 生产全业务验证 v2（真实网关；回调自签与网关密码学同构）
# 修正：sync 先于订阅检查；agents/all 路径；空响应自动重试抗网络抖动
set -uo pipefail
API=https://manageapi.xapptool.cn
USERAPI=https://workapi.xapptool.cn
EPAY_KEY='pD7J5Cz5755oFp5o0AaAhzaAnp45En4O'
PASS=0; FAIL=0
ck() { if [ "$2" = "0" ]; then PASS=$((PASS+1)); echo "  [OK] $1"; else FAIL=$((FAIL+1)); echo "  [X] $1"; fi }
md5hex() { if command -v md5sum >/dev/null 2>&1; then md5sum | cut -d' ' -f1; else md5 -q; fi }
# curl 带重试：网络抖动下空响应自动重试 3 感
rcurl() { # rcurl <tries> <curl args...>
  local n=$1; shift; local out=""
  for i in $(seq 1 $n); do
    out=$(curl -s --max-time 25 "$@") && [ -n "$out" ] && { echo "$out"; return 0; }
    sleep 2
  done
  echo "$out"; return 1
}

echo "===== S0 公开配置 ====="
CFG=$(rcurl 3 $USERAPI/api/public/config)
echo "$CFG" | grep -q '"edition":"saas"'; ck "edition=saas" $?
echo "$CFG" | grep -q '"tenant_mode":true'; ck "tenant_mode=true" $?

echo "===== S1 注册新租户 ====="
TS=$(date +%s)
REG=$(rcurl 3 -X POST $USERAPI/api/tenant/register -H 'Content-Type: application/json' -d "{
  \"name\":\"E2E验证商户${TS}\",\"username\":\"e2eadm${TS}\",
  \"email\":\"e2e${TS}@test.local\",\"password\":\"E2e#Pass${TS}\",
  \"company_name\":\"E2E验证商户${TS}\",\"legal_person\":\"验证员\",
  \"phone\":\"13900139000\",\"address\":\"验证路1号\",\"license_no\":\"E2E-LIC-${TS}\",
  \"regulatory_ack\":true,\"agreement_version\":\"2026-01-01\"}")
echo "$REG" | head -c 200; echo
echo "$REG" | grep -qE '"code":0'; ck "注册 code=0" $?
ADMIN_USER="e2eadm${TS}"; ADMIN_PASS="E2e#Pass${TS}"

echo "===== S2 登录 + 首次 sync（把平台 trial 镜像拉下来） ====="
LOGIN=$(rcurl 3 -X POST $USERAPI/api/auth/login -H 'Content-Type: application/json' \
  -d "{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
TOKEN=$(echo "$LOGIN" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p' | head -1)
[ -n "$TOKEN" ]; ck "登录 JWT" $?
AUTH="Authorization: Bearer $TOKEN"
sleep 3   # 等注册异步上报平台完成（trial 创建）
SYNC=$(rcurl 3 -X POST -H "$AUTH" $USERAPI/api/modules/sync)
echo "$SYNC" | head -c 150; echo
echo "$SYNC" | grep -qE '"code":0'; ck "sync code=0" $?
echo "$SYNC" | grep -q '"degraded":false'; ck "sync 非降级（平台可达）" $?

echo "===== S3 trial 状态 ====="
SUB=$(rcurl 3 -H "$AUTH" $USERAPI/api/subscription/status)
echo "$SUB" | head -c 250; echo
echo "$SUB" | grep -q '"status":"trial"'; ck "订阅=trial" $?
echo "$SUB" | grep -q '"price_cents":14900'; ck "续期定价=14900" $?
echo "$SUB" | grep -q '"locked":false'; ck "trial 未锁定" $?

echo "===== S4 开户赠送 ====="
MY=$(rcurl 3 -H "$AUTH" $USERAPI/api/modules/my)
echo "$MY" | grep -q '"module_key":"browser-automation"[^}]*"status":"active"'; B=$?; [ $B -ne 0 ]; ck "browser-automation 未默认激活" $?
AGENTS=$(rcurl 3 -H "$AUTH" $USERAPI/api/agents/all)
echo "$AGENTS" | grep -q '"code":0'; ck "agents 路由连通（赠送由 starter-kit 生日志核实：克隆+资产包）" $?

echo "===== S5 市场价格 ====="
MKT=$(rcurl 3 $USERAPI/api/module-market/list)
echo "$MKT" | grep -q '"module_key":"browser-automation"[^}]*"price_cents":9900'; ck "浏览器自动化 ¥99" $?
FCNT=$(echo "$MKT" | grep -o '"price_cents":0' | wc -l | tr -d ' ')
[ "$FCNT" -ge 15 ]; ck "免费模块 >=15 (实际 $FCNT)" $?

echo "===== S6 下单 → 真实网关 pay_url ====="
ORDER=$(rcurl 3 -X POST $USERAPI/api/module-market/order -H "$AUTH" -H 'Content-Type: application/json' -d '{
  "module_keys":["browser-automation"],
  "contact_name":"验证员","contact_phone":"13900139000","contact_email":"e2e@test.local"}')
echo "$ORDER" | head -c 250; echo
ORD_NO=$(echo "$ORDER" | sed -n 's/.*"order_no":"\([^"]*\)".*/\1/p' | head -1)
PAY_URL=$(echo "$ORDER" | sed -n 's/.*"pay_url":"\([^"]*\)".*/\1/p' | head -1)
[ -n "$ORD_NO" ]; ck "订单 $ORD_NO" $?
[ -n "$PAY_URL" ]; ck "pay_url 下发" $?
echo "$PAY_URL" | grep -q 'gggua.com'; ck "真实网关 pay_url" $?

echo "===== S7 自签回调 → 激活 ====="
SIGN=$(printf '%s' "money=99.00&name=mock&out_trade_no=${ORD_NO}&pid=1295&trade_no=E2E${TS}&trade_status=TRADE_SUCCESS&type=alipay&key=${EPAY_KEY}" | md5hex)
NOTIFY=$(rcurl 3 -G "$API/public/payments/notify/epay" \
  --data-urlencode "pid=1295" --data-urlencode "trade_no=E2E${TS}" \
  --data-urlencode "out_trade_no=${ORD_NO}" --data-urlencode "type=alipay" \
  --data-urlencode "name=mock" --data-urlencode "money=99.00" \
  --data-urlencode "trade_status=TRADE_SUCCESS" --data-urlencode "sign=${SIGN}" \
  --data-urlencode "sign_type=MD5")
[ "$NOTIFY" = "success" ]; ck "回调 success" $?
sleep 2
SYNC2=$(rcurl 3 -X POST -H "$AUTH" $USERAPI/api/modules/sync)
MY2=$(rcurl 3 -H "$AUTH" $USERAPI/api/modules/my)
echo "$MY2" | grep -q '"module_key":"browser-automation"[^}]*"status":"active"'; ck "浏览器自动化已激活" $?

echo "===== S8 续期下单 + 回调 → active ====="
REN=$(rcurl 3 -X POST $USERAPI/api/subscription/renew -H "$AUTH" -H 'Content-Type: application/json' \
  -d '{"contact_name":"验证员","contact_phone":"13900139000"}')
echo "$REN" | head -c 200; echo
echo "$REN" | grep -q '"total_amount_cents":14900'; ck "续期金额=14900" $?
REN_NO=$(echo "$REN" | sed -n 's/.*"order_no":"\([^"]*\)".*/\1/p' | head -1)
[ -n "$REN_NO" ]; ck "续期单 $REN_NO" $?
SIGN2=$(printf '%s' "money=149.00&name=mock&out_trade_no=${REN_NO}&pid=1295&trade_no=E2ERT${TS}&trade_status=TRADE_SUCCESS&type=alipay&key=${EPAY_KEY}" | md5hex)
NOTIFY2=$(rcurl 3 -G "$API/public/payments/notify/epay" \
  --data-urlencode "pid=1295" --data-urlencode "trade_no=E2ERT${TS}" \
  --data-urlencode "out_trade_no=${REN_NO}" --data-urlencode "type=alipay" \
  --data-urlencode "name=mock" --data-urlencode "money=149.00" \
  --data-urlencode "trade_status=TRADE_SUCCESS" --data-urlencode "sign=${SIGN2}" \
  --data-urlencode "sign_type=MD5")
[ "$NOTIFY2" = "success" ]; ck "续期回调 success" $?
sleep 2
rcurl 3 -X POST -H "$AUTH" $USERAPI/api/modules/sync >/dev/null
SUB2=$(rcurl 3 -H "$AUTH" $USERAPI/api/subscription/status)
echo "$SUB2" | head -c 250; echo
echo "$SUB2" | grep -q '"status":"active"'; ck "续期后=active" $?
echo "$SUB2" | grep -q '"locked":false'; ck "未锁定" $?

echo "===== S9 平台端复核 ====="
PT=$(rcurl 3 -X POST $API/api/auth/login -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"Admin123"}' | sed -n 's/.*"token":"\([^"]*\)".*/\1/p' | head -1)
P1=$(rcurl 3 -H "Authorization: Bearer $PT" "$API/platform/orders/${ORD_NO}")
echo "$P1" | grep -qE '"status":"(paid|fulfilled)"'; ck "模块单 fulfilled" $?
P2=$(rcurl 3 -H "Authorization: Bearer $PT" "$API/platform/orders/${REN_NO}")
echo "$P2" | grep -qE '"status":"(paid|fulfilled)"'; ck "续期单 fulfilled" $?

echo ""
echo "========================================"
echo "  PASS=$PASS  FAIL=$FAIL"
echo "  测试商户 admin=$ADMIN_USER"
echo "  模块单=$ORD_NO 续期单=$REN_NO"
echo "========================================"
[ "$FAIL" = "0" ]

#!/bin/bash
# 清理「单个 QA 租户」在生产库里的全部落库数据（user_db 与 platform_db 两相）。
#
# 为什么需要它：线上没有租户删除接口（只有用户/商户/模块/套餐的删除），而自动化验证
# 每跑一次就会注册一个租户，涉及 300 多张带 tenant_id 的表。逐表手写 DELETE 会随
# schema 漂移，所以按 information_schema 现取列清单生成语句，并且只删一个租户的数据。
#
# 三条前置护栏（缺一条即整体中止，不提交）：
#   1) 目标租户必须命中测试/演示签名：license_no 前缀 QA- / E2E-LIC-，或邮箱落在
#      保留域 @qa.example / @test.local。真实客户不会命中，因此不可能被本脚本命中。
#   2) 租户 id 不得落在保留名单（默认 1：本地自管实例的租户），code 不得以 local- 开头。
#   3) 默认 dry-run：整相在事务里真删（所以外键/类型问题会暴露），最后 ROLLBACK；
#      必须显式 --commit 才落盘。
#   4) 平台侧归属不自相矛盾：补认到的单若挂着别的非空 tenant_code，整体中止。
#
# 平台侧清理两道认领通道：
#   1) tenant_code 精确归属（续费单、LLM 充值单一贯带这个字段）；
#   2) 本地镜像补认——模块下单路径在 2026-10-20 之前不传 tenant_code，平台侧那
#      些行落库是空串，按 code 永远认领不到。用户端 module_orders 里有同一批
#      order_no，于是开第二道通道：先按本租户的镜像 order_no 补认，且只接受
#      平台侧 tenant_code 为空串的行；若同一 order_no 在平台侧挂着别的非空
#      code，判定归属账目冲突、整体中止（宁可不清理，也不替别人的租户删单）。
# 商户/装机级表（merchants、platform_installs、platform_heartbeats、api_logs、
# usage_reports、sync_logs、tenant_modules 的 default_base 行）属部署级共享数据，
# 一个租户的清理不得触碰。
#
# 跑的顺序是「平台相在前、用户相在后」：镜像 order_no 是唯一能空串行的线索，
# 用户相先提交就会把这条线索删掉——万一平台相随后失败，那些空串单再也认不回来。
#
# 口令不落地：DB 口令在生产机上由 docker inspect 现取，脚本只透传 SQL，不打印口令。
#
# 用法：
#   scripts/cleanup_qa_tenants.sh --tenant-code QA20261020153000           # dry-run
#   scripts/cleanup_qa_tenants.sh --tenant-code QA20261020153000 --commit  # 真删
#   QA_PROD_SSH=root@1.2.3.4:2222 scripts/cleanup_qa_tenants.sh --tenant-code X --commit
#   scripts/cleanup_qa_tenants.sh --tenant-code X --phase platform         # 只跑平台相
set -uo pipefail

QA_PROD_SSH="${QA_PROD_SSH:-}"
USER_PG_CONTAINER="${USER_PG_CONTAINER:-mtk-postgres}"
USER_PG_DB="${USER_PG_DB:-user_db}"
USER_PG_PORT="${USER_PG_PORT:-8202}"
USER_PG_USER="${USER_PG_USER:-admin}"
PF_PG_CONTAINER="${PF_PG_CONTAINER:-mtk-platform-postgres}"
PF_PG_DB="${PF_PG_DB:-platform_db}"
PF_PG_PORT="${PF_PG_PORT:-8201}"
PF_PG_USER="${PF_PG_USER:-admin}"
# 本地自管实例的租户 id：由部署时生成，不允许被本脚本清理
RESERVED_IDS="${RESERVED_IDS:-1}"

COMMIT=0
TENANT_CODE=""
PHASE="all"
while [ $# -gt 0 ]; do
  case "$1" in
    --tenant-code) TENANT_CODE="${2:-}"; shift 2 ;;
    --phase)       PHASE="${2:-}"; shift 2 ;;
    --commit)      COMMIT=1; shift ;;
    -h|--help)     sed -n '2,36p' "$0"; exit 0 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

[ -n "$TENANT_CODE" ] || { echo "必须给 --tenant-code" >&2; exit 2; }
case "$TENANT_CODE" in
  *[!A-Za-z0-9_-]*) echo "--tenant-code 只允许字母、数字、下划线、短横线" >&2; exit 2 ;;
esac
case "$RESERVED_IDS" in
  *[!0-9,]*) echo "RESERVED_IDS 只允许数字与逗号" >&2; exit 2 ;;
esac
case "$PHASE" in
  all|user|platform) ;;
  *) echo "--phase 只支持 all|user|platform" >&2; exit 2 ;;
esac
[ -n "$QA_PROD_SSH" ] || { echo "请设 QA_PROD_SSH=<user@host>[:<port>]（生产机 SSH 目标）" >&2; exit 2; }

SSH_HOST="${QA_PROD_SSH%%:*}"
SSH_PORT="${QA_PROD_SSH#*:}"
if [ "$SSH_PORT" = "$QA_PROD_SSH" ]; then SSH_PORT=22; fi
ssh_run() { ssh -p "$SSH_PORT" -o StrictHostKeyChecking=no -o ConnectTimeout=15 "$SSH_HOST" "$@"; }

# 模板用带引号 heredoc 写（不做本地展开），占位符再单独替换，避免 $ 转义踩坑
TOKEN_CODE='__CODE__'
TOKEN_RESERVED='__RESERVED__'
TOKEN_EXTRA='__EXTRA__'
# 默认给一枚空串值：VALUES 装架照常建，谓词把它滤掉，补认通道就是 0 行
EXTRA_VALUES="('')"

TMP="${TMPDIR:-/tmp}"
USER_SQL="$TMP/qa_cleanup_user_$$.sql"
PF_SQL="$TMP/qa_cleanup_pf_$$.sql"
trap 'rm -f "$USER_SQL" "$PF_SQL"' EXIT

if [ "$COMMIT" = "1" ]; then
  TAIL="COMMIT;
SELECT 'PHASE_COMMITTED' AS result;"
else
  TAIL="ROLLBACK;
SELECT 'PHASE_DRYRUN_ROLLED_BACK' AS result;"
fi

# ── user_db 相 ────────────────────────────────────────────────────────────
build_user_sql() {
cat <<'TPL'
\set ON_ERROR_STOP on
\pset footer off
BEGIN;

-- 1) 目标租户定位 + 签名/保留名单护栏
CREATE TEMP TABLE qa_target(id bigint PRIMARY KEY, code text NOT NULL, license_no text, email text);
INSERT INTO qa_target
SELECT t.id, t.code, coalesce(t.license_no,''), coalesce(t.email,'')
  FROM tenants t
 WHERE t.code = '__CODE__';

DO $$
DECLARE n bigint;
BEGIN
  SELECT count(*) INTO n FROM qa_target;
  IF n <> 1 THEN
    RAISE EXCEPTION '租户 __CODE__ 在 tenants 里不存在（n=%），拒绝继续', n;
  END IF;
  SELECT count(*) INTO n FROM qa_target
   WHERE license_no LIKE 'QA-%' OR license_no LIKE 'E2E-LIC-%'
      OR email LIKE '%@qa.example' OR email LIKE '%@test.local';
  IF n <> 1 THEN
    RAISE EXCEPTION '护栏1 失败：租户 __CODE__ 不命中测试/演示签名（license_no 前缀 QA-/E2E-LIC-，邮箱 @qa.example/@test.local），判定为真实客户数据，拒绝清理';
  END IF;
  SELECT count(*) INTO n FROM qa_target WHERE code LIKE 'local-%' OR id IN (__RESERVED__);
  IF n <> 0 THEN
    RAISE EXCEPTION '护栏2 失败：目标租户命中保留名单（code local-* 或 id ∈ (__RESERVED__)），拒绝清理';
  END IF;
  RAISE NOTICE '护栏1/2 通过：目标租户 __CODE__ (id=%) 命中测试签名', (SELECT id FROM qa_target);
END
$$;

-- 2) 现取「含 tenant_id 的基表」清单（漂移安全：列改名/新表自动进出）
CREATE TEMP TABLE qa_cols(tbl text PRIMARY KEY, pred text NOT NULL);
INSERT INTO qa_cols
SELECT c.table_name,
  CASE WHEN c.data_type IN ('bigint','integer','smallint','numeric')
       THEN format('%I = (SELECT id FROM qa_target)', c.column_name)
       ELSE format('%I::text = (SELECT id::text FROM qa_target)', c.column_name) END
  FROM information_schema.columns c
  JOIN information_schema.tables t
    ON t.table_schema = c.table_schema AND t.table_name = c.table_name
 WHERE c.table_schema = 'public' AND c.column_name = 'tenant_id'
   AND t.table_type = 'BASE TABLE' AND c.table_name <> 'tenants'
   AND c.data_type IN ('bigint','integer','smallint','numeric','text','character varying');

-- tenants 行本身也纳入同一套装架（删除谓词自带签名，双保险）
INSERT INTO qa_cols
SELECT 'tenants', format(
  'code = %L AND (license_no LIKE %L OR license_no LIKE %L OR email LIKE %L OR email LIKE %L)',
  '__CODE__', 'QA-%', 'E2E-LIC-%', '%@qa.example', '%@test.local');

DO $$
BEGIN
  IF (SELECT count(*) FROM qa_cols) < 50 THEN
    RAISE EXCEPTION '护栏3 失败：动态探针只覆盖 % 张表，而 user_db 有 340+ 张带 tenant_id 的基表——判定列名口径已漂移，拒绝清理',
      (SELECT count(*) FROM qa_cols);
  END IF;
  RAISE NOTICE '护栏3 通过：探针覆盖表数 = %', (SELECT count(*) FROM qa_cols);
END
$$;

-- 3) 删除前逐表计数
CREATE TEMP TABLE qa_before(tbl text PRIMARY KEY, n bigint NOT NULL);
SELECT format($f$INSERT INTO qa_before SELECT %L, count(*) FROM %I WHERE %s
             ON CONFLICT (tbl) DO UPDATE SET n = EXCLUDED.n;$f$, q.tbl, q.tbl, q.pred)
  FROM qa_cols q ORDER BY q.tbl
\gexec
SELECT '### user_db 删除前非零表 ###' AS chk;
SELECT tbl, n FROM qa_before WHERE n > 0 ORDER BY n DESC, tbl;
SELECT '### user_db 删除前合计 ###' AS chk,
       (SELECT coalesce(sum(n),0) FROM qa_before) AS rows_to_delete,
       (SELECT count(*) FROM qa_before)           AS probe_tables;

-- 4) 执行删除（DELETE ... RETURNING 计数入账，与删除前逐表对账）
CREATE TEMP TABLE qa_deleted(tbl text PRIMARY KEY, n bigint NOT NULL);
SELECT format($f$WITH d AS (DELETE FROM %I WHERE %s RETURNING 1)
             INSERT INTO qa_deleted SELECT %L, count(*) FROM d;$f$,
         q.tbl, q.pred, q.tbl)
  FROM qa_cols q ORDER BY q.tbl
\gexec
SELECT '### user_db 实际删除非零表 ###' AS chk;
SELECT tbl, n FROM qa_deleted WHERE n > 0 ORDER BY n DESC, tbl;

-- 5) 归属一致性探针：emb_tenant_id 是 rag_products 上的第二个租户维度，
--    若它与 tenant_id 指向不同租户，说明「按 tenant_id 删」不等于「删这个租户的数据」
CREATE TEMP TABLE qa_cross(n bigint NOT NULL);
SELECT format($f$INSERT INTO qa_cross SELECT count(*) FROM %I x
             WHERE x.emb_tenant_id = (SELECT id FROM qa_target)
               AND x.tenant_id IS DISTINCT FROM (SELECT id FROM qa_target);$f$, c.table_name)
  FROM information_schema.columns c
 WHERE c.table_schema = 'public' AND c.column_name = 'emb_tenant_id'
   AND EXISTS (SELECT 1 FROM information_schema.columns k
                WHERE k.table_schema='public' AND k.table_name = c.table_name
                  AND k.column_name = 'tenant_id')
\gexec
DO $$
BEGIN
  IF (SELECT coalesce(sum(n),0) FROM qa_cross) <> 0 THEN
    RAISE EXCEPTION '护栏4 失败：% 行的 emb_tenant_id 命中目标但 tenant_id 不是目标——归属口径不可信，拒绝清理',
      (SELECT coalesce(sum(n),0) FROM qa_cross);
  END IF;
  RAISE NOTICE '护栏4 通过：emb_tenant_id 与 tenant_id 归属一致（探针 % 张表）', (SELECT count(*) FROM qa_cross);
END
$$;

-- 6) 残留复扫 + 对账（同一套装架，删除后必须全 0，且删除数=删除前数）
CREATE TEMP TABLE qa_resid(tbl text PRIMARY KEY, n bigint NOT NULL);
SELECT format($f$INSERT INTO qa_resid SELECT %L, count(*) FROM %I WHERE %s
             ON CONFLICT (tbl) DO UPDATE SET n = EXCLUDED.n;$f$, q.tbl, q.tbl, q.pred)
  FROM qa_cols q ORDER BY q.tbl
\gexec
DO $$
DECLARE b bigint; dl bigint; r bigint; pc bigint; rc bigint; cc bigint; dn bigint;
BEGIN
  SELECT coalesce(sum(n),0) INTO b FROM qa_before;
  SELECT coalesce(sum(n),0) INTO dl FROM qa_deleted;
  SELECT coalesce(sum(n),0) INTO r FROM qa_resid;
  SELECT count(*) INTO pc FROM qa_before;
  SELECT count(*) INTO rc FROM qa_resid;
  SELECT count(*) INTO cc FROM qa_cols;
  SELECT count(*) INTO dn FROM qa_deleted;
  -- 装架自证：三套计数必须各自覆盖全部探针表，否则「0 行」只是没数过
  IF pc <> cc OR dn <> cc OR rc <> cc THEN
    RAISE EXCEPTION '装架自证失败：探针 % 张表，删除前 % 张、已删 % 张、残留 % 张——计数未跑满，读数不可信',
      cc, pc, dn, rc;
  END IF;
  IF dl <> b THEN
    RAISE EXCEPTION '对账失败：删除 % 行 != 删除前 % 行', dl, b;
  END IF;
  IF r <> 0 THEN
    RAISE EXCEPTION '残留 % 行未删（见 qa_resid 非零项），中止', r;
  END IF;
  IF EXISTS (SELECT 1 FROM qa_resid WHERE n > 0) THEN
    RAISE EXCEPTION '存在非零残留表';
  END IF;
  RAISE NOTICE '对账通过：% 张探针表全部跑满，删除 % 行 == 删除前 % 行，残留 0', cc, dl, b;
END
$$;

SELECT '### user_db 清理后保留面 ###' AS chk,
       (SELECT count(*) FROM tenants)      AS tenants,
       (SELECT count(*) FROM system_users) AS users,
       (SELECT count(*) FROM tenant_modules) AS tenant_modules;
TPL
}

# ── platform_db 相 ───────────────────────────────────────────────────────
build_pf_sql() {
cat <<'TPL'
\set ON_ERROR_STOP on
\pset footer off
BEGIN;

-- 1) 清理对象清单：表 + 归属键（漂移护栏，缺列即中止）
CREATE TEMP TABLE pf_req(tbl text NOT NULL, col text NOT NULL, PRIMARY KEY (tbl, col));
INSERT INTO pf_req VALUES
  ('orders','tenant_code'), ('orders','id'), ('orders','order_no'), ('orders','merchant_key'),
  ('tenant_registrations','tenant_code'), ('tenant_subscriptions','tenant_code'),
  ('order_items','order_id'), ('order_status_logs','order_id'), ('payment_records','order_id'),
  ('payment_callbacks','order_no'), ('payment_callbacks','trade_no'),
  ('subscription_grants','order_no'),
  ('tenant_modules','order_id'), ('tenant_modules','source'),
  ('purchases','tenant_code'), ('purchases','asset_id'),
  ('revenue_records','purchase_id'), ('revenue_records','contributor_id'),
  ('credit_logs','tenant_code'), ('merchant_credits','tenant_code'),
  ('llm_wallets','tenant_code'), ('llm_wallet_txns','tenant_code'),
  ('llm_usage_records','tenant_code'),
  ('assets','purchase_count'), ('assets','download_count'),
  ('contributors','total_sales'), ('contributors','balance'), ('contributors','total_revenue');
DO $$
DECLARE miss text;
BEGIN
  SELECT string_agg(r.tbl||'.'||r.col, ', ' ORDER BY r.tbl||'.'||r.col) INTO miss
    FROM pf_req r
   WHERE NOT EXISTS (SELECT 1 FROM information_schema.columns c
                      WHERE c.table_schema='public' AND c.table_name=r.tbl AND c.column_name=r.col);
  IF miss IS NOT NULL THEN
    RAISE EXCEPTION '平台侧 schema 漂移，缺少列：%', miss;
  END IF;
  RAISE NOTICE '平台侧护栏通过：% 个归属键全部在场', (SELECT count(*) FROM pf_req);
END
$$;

-- 2) 该租户在平台侧的订单集合（id 与 order_no 两份键都要用）。
--    通道 A：tenant_code 精确归属；
--    通道 B：本地镜像 order_no 补认——只接受平台侧 tenant_code 为空串的行，
--            那正是模块下单路径漏传字段留下的形态。
CREATE TEMP TABLE pf_extra(order_no text PRIMARY KEY);
INSERT INTO pf_extra SELECT order_no FROM (VALUES __EXTRA__) AS v(order_no) WHERE order_no <> '';

DO $$
DECLARE c bigint;
BEGIN
  SELECT count(*) INTO c FROM orders o
    JOIN pf_extra e ON e.order_no = o.order_no
   WHERE coalesce(o.tenant_code, '') NOT IN ('', '__CODE__');
  IF c <> 0 THEN
    RAISE EXCEPTION '护栏4 失败：% 枚镜像 order_no 在平台侧归属到别的非空 tenant_code——两本账互相矛盾，拒绝清理', c;
  END IF;
END
$$;

CREATE TEMP TABLE pf_orders(id text PRIMARY KEY, order_no text NOT NULL,
                            merchant_key text NOT NULL, claimed_by text NOT NULL);
INSERT INTO pf_orders
SELECT id, order_no, merchant_key, 'tenant_code' FROM orders WHERE tenant_code = '__CODE__'
UNION
SELECT o.id, o.order_no, o.merchant_key, 'local_mirror'
  FROM orders o JOIN pf_extra e ON e.order_no = o.order_no
 WHERE coalesce(o.tenant_code, '') = '';

-- 资产购买集合：只删能按 tenant_code 归属的行，并据此反算资产/作者计数
CREATE TEMP TABLE pf_purch(id bigint PRIMARY KEY, asset_id bigint NOT NULL,
                           contributor_id bigint, contributor_amount numeric(12,2));
INSERT INTO pf_purch
SELECT p.id, p.asset_id, a.contributor_id, coalesce(r.contributor_amount, 0)
  FROM purchases p
  JOIN assets a ON a.id = p.asset_id
  LEFT JOIN revenue_records r ON r.purchase_id = p.id
 WHERE p.tenant_code = '__CODE__';

DO $$
BEGIN
  IF (SELECT count(*) FROM tenant_registrations WHERE tenant_code='__CODE__') = 0
     AND (SELECT count(*) FROM tenant_subscriptions WHERE tenant_code='__CODE__') = 0
     AND (SELECT count(*) FROM pf_orders) = 0
     AND (SELECT count(*) FROM pf_purch) = 0
     AND (SELECT count(*) FROM payment_callbacks WHERE order_no='' AND trade_no LIKE 'QA%') = 0 THEN
    RAISE EXCEPTION '平台侧不存在租户 __CODE__ 的任何注册/订阅/订单/购买行——本相无可清理（若确属预期，用 --phase user 跳过）';
  END IF;
  RAISE NOTICE '平台侧目标：订单 % 笔（tenant_code % 笔、镜像补认空串 % 笔），资产购买 % 笔',
    (SELECT count(*) FROM pf_orders),
    (SELECT count(*) FROM pf_orders WHERE claimed_by = 'tenant_code'),
    (SELECT count(*) FROM pf_orders WHERE claimed_by = 'local_mirror'),
    (SELECT count(*) FROM pf_purch);
  -- 联表后行数必须等量，否则「按 tenant_code 删掉的购买」有反算不到的漏项
  IF (SELECT count(*) FROM pf_purch) <> (SELECT count(*) FROM purchases WHERE tenant_code='__CODE__') THEN
    RAISE EXCEPTION 'purchases 联表后少 % 行（资产或作者行缺失）——删除集合无法同源反算计数，中止',
      (SELECT count(*) FROM purchases WHERE tenant_code='__CODE__') - (SELECT count(*) FROM pf_purch);
  END IF;
END
$$;

-- 3) 删除计划：一张有序清单同时驱动「删除前计数 / 删除 / 残留复扫」三套装架，
--    三个读数数的是同一批谓词——不留三份真相
CREATE TEMP TABLE pf_plan(ord integer PRIMARY KEY, tbl text NOT NULL,
                          label text NOT NULL, pred text NOT NULL);
INSERT INTO pf_plan VALUES
  ( 1,'order_items',         'order_items[order_id]',
        'order_id IN (SELECT id FROM pf_orders)'),
  ( 2,'order_status_logs',   'order_status_logs[order_id]',
        'order_id IN (SELECT id FROM pf_orders)'),
  ( 3,'payment_records',     'payment_records[order_id]',
        'order_id IN (SELECT id FROM pf_orders)'),
  ( 4,'payment_callbacks',   'payment_callbacks[order_no]',
        'order_no IN (SELECT order_no FROM pf_orders)'),
  ( 5,'subscription_grants', 'subscription_grants[order_no]',
        'order_no IN (SELECT order_no FROM pf_orders)'),
  ( 6,'tenant_modules',      'tenant_modules[order_id, 非 default_base]',
        'order_id IN (SELECT id FROM pf_orders) AND source <> ''default_base'''),
  ( 7,'llm_usage_records',   'llm_usage_records[tenant_code]',
        'tenant_code = ''__CODE__'''),
  ( 8,'llm_wallet_txns',     'llm_wallet_txns[tenant_code]',
        'tenant_code = ''__CODE__'''),
  ( 9,'llm_wallets',         'llm_wallets[tenant_code]',
        'tenant_code = ''__CODE__'''),
  (10,'merchant_credits',    'merchant_credits[tenant_code]',
        'tenant_code = ''__CODE__'''),
  (11,'credit_logs',         'credit_logs[tenant_code]',
        'tenant_code = ''__CODE__'''),
  (12,'revenue_records',     'revenue_records[purchase_id]',
        'purchase_id IN (SELECT id FROM pf_purch)'),
  (13,'purchases',           'purchases[tenant_code]',
        'tenant_code = ''__CODE__'''),
  -- 回调落库时订单已不可解析的行（order_no 为空）只能按 QA trade_no 前缀认领
  (14,'payment_callbacks',   'payment_callbacks[空 order_no, QA trade_no]',
        'order_no = '''' AND trade_no LIKE ''QA%'''),
  (15,'orders',              'orders[tenant_code]',
        'tenant_code = ''__CODE__'''),
  (16,'tenant_subscriptions','tenant_subscriptions[tenant_code]',
        'tenant_code = ''__CODE__'''),
  (17,'tenant_registrations','tenant_registrations[tenant_code]',
        'tenant_code = ''__CODE__''');

CREATE TEMP TABLE pf_before(ord integer PRIMARY KEY, label text NOT NULL, n bigint NOT NULL);
SELECT format($f$INSERT INTO pf_before SELECT %L, %L, count(*) FROM %I WHERE %s;$f$,
              p.ord, p.label, p.tbl, p.pred)
  FROM pf_plan p ORDER BY p.ord
\gexec
SELECT '### platform_db 删除前 ###' AS chk;
SELECT label, n FROM pf_before WHERE n > 0 ORDER BY ord;
SELECT '### platform_db 删除前合计 ###' AS chk,
       (SELECT coalesce(sum(n),0) FROM pf_before) AS rows_to_delete,
       (SELECT count(*) FROM pf_before)           AS probe_items;

-- 4) 删除（按计划顺序逐条执行；DELETE ... RETURNING 计数入账，与删除前对账）
CREATE TEMP TABLE pf_deleted(ord integer PRIMARY KEY, label text NOT NULL, n bigint NOT NULL);
SELECT format($f$WITH d AS (DELETE FROM %I WHERE %s RETURNING 1)
             INSERT INTO pf_deleted SELECT %L, %L, count(*) FROM d;$f$,
         p.tbl, p.pred, p.ord, p.label)
  FROM pf_plan p ORDER BY p.ord
\gexec
SELECT '### platform_db 实际删除 ###' AS chk;
SELECT label, n FROM pf_deleted WHERE n > 0 ORDER BY ord;

-- 5) 反算被删购买造成的资产/作者计数（与购买流水同源，不写死数字）
UPDATE assets a
   SET purchase_count = a.purchase_count - d.n,
       download_count = a.download_count - d.n,
       updated_at = NOW()
  FROM (SELECT asset_id, count(*) AS n FROM pf_purch GROUP BY asset_id) d
 WHERE a.id = d.asset_id;

UPDATE contributors c
   SET total_sales   = c.total_sales - d.n,
       balance       = c.balance - d.amt,
       total_revenue = c.total_revenue - d.amt,
       updated_at    = NOW()
  FROM (SELECT contributor_id, count(*) AS n, sum(contributor_amount) AS amt
          FROM pf_purch WHERE contributor_id IS NOT NULL GROUP BY contributor_id) d
 WHERE c.id = d.contributor_id;

-- 6) 残留复扫：与删除计划同一批谓词，逐条重数
CREATE TEMP TABLE pf_resid(ord integer PRIMARY KEY, label text NOT NULL, n bigint NOT NULL);
SELECT format($f$INSERT INTO pf_resid SELECT %L, %L, count(*) FROM %I WHERE %s;$f$,
              p.ord, p.label, p.tbl, p.pred)
  FROM pf_plan p ORDER BY p.ord
\gexec

DO $$
DECLARE pc bigint; bc bigint; dc bigint; rc bigint;
        b bigint; dl bigint; r bigint;
        o bigint; neg bigint; iv bigint;
BEGIN
  -- 装架自证：三套计数都要覆盖整张计划表，否则「0 行」只是没数过
  SELECT count(*) INTO pc FROM pf_plan;
  SELECT count(*) INTO bc FROM pf_before;
  SELECT count(*) INTO dc FROM pf_deleted;
  SELECT count(*) INTO rc FROM pf_resid;
  IF pc <> bc OR pc <> dc OR pc <> rc THEN
    RAISE EXCEPTION '装架自证失败：计划 % 条，删除前 % 条、已删 % 条、残留 % 条——计数未跑满，读数不可信', pc, bc, dc, rc;
  END IF;
  SELECT coalesce(sum(n),0) INTO b  FROM pf_before;
  SELECT coalesce(sum(n),0) INTO dl FROM pf_deleted;
  SELECT coalesce(sum(n),0) INTO r  FROM pf_resid;
  IF dl <> b THEN RAISE EXCEPTION '平台侧对账失败：删除 % 行 != 删除前 % 行', dl, b; END IF;
  IF r  <> 0 THEN RAISE EXCEPTION '平台侧残留 % 行未删（见 pf_resid 非零项），中止', r; END IF;

  o := (SELECT count(*) FROM order_items oi WHERE NOT EXISTS (SELECT 1 FROM orders oo WHERE oo.id=oi.order_id))
     + (SELECT count(*) FROM order_status_logs l WHERE NOT EXISTS (SELECT 1 FROM orders oo WHERE oo.id=l.order_id))
     + (SELECT count(*) FROM payment_records pr WHERE NOT EXISTS (SELECT 1 FROM orders oo WHERE oo.id=pr.order_id))
     + (SELECT count(*) FROM subscription_grants g WHERE NOT EXISTS (SELECT 1 FROM orders oo WHERE oo.order_no=g.order_no))
     + (SELECT count(*) FROM revenue_records rr WHERE NOT EXISTS (SELECT 1 FROM purchases p WHERE p.id=rr.purchase_id))
     + (SELECT count(*) FROM tenant_modules m WHERE m.source <> 'default_base'
        AND NOT EXISTS (SELECT 1 FROM orders oo WHERE oo.id=m.order_id));
  IF o <> 0 THEN RAISE EXCEPTION '平台侧清理后出现 % 行孤儿（父表被删而子表留着），中止', o; END IF;

  neg := (SELECT count(*) FROM assets WHERE purchase_count < 0 OR download_count < 0)
       + (SELECT count(*) FROM contributors WHERE total_sales < 0 OR balance < 0 OR total_revenue < 0);
  IF neg <> 0 THEN RAISE EXCEPTION '反算后出现 % 处负计数，说明删除集合与流水不同源，中止', neg; END IF;

  iv := (SELECT count(*) FROM contributors c
          WHERE c.total_sales <> coalesce((SELECT sum(a.purchase_count) FROM assets a
                                           WHERE a.contributor_id = c.id), 0));
  IF iv <> 0 THEN RAISE EXCEPTION '不变量破裂：% 位作者的 total_sales 与资产 purchase_count 之和不一致，中止', iv; END IF;

  RAISE NOTICE '平台侧复扫通过：% 条计划全部跑满，删除 % 行 == 删除前 % 行，残留 0、孤儿 0、负计数 0、作者计数不变量成立',
    pc, dl, b;
END
$$;

SELECT '### platform_db 清理后保留面 ###' AS chk,
       (SELECT count(*) FROM merchants)            AS merchants,
       (SELECT count(*) FROM platform_installs)    AS installs,
       (SELECT count(*) FROM tenant_modules)       AS tenant_modules,
       (SELECT count(*) FROM assets)               AS assets,
       (SELECT count(*) FROM contributors)         AS contributors,
       (SELECT count(*) FROM tenant_subscriptions) AS subscriptions;
TPL
}

run_phase() { # run_phase <label> <container> <db> <port> <user> <local_sql>
  local label="$1" container="$2" dbname="$3" port="$4" dbuser="$5" sql="$6"
  echo "== ${label} 相：$( [ "$COMMIT" = "1" ] && echo 'COMMIT（真删）' || echo 'DRY-RUN（事务内执行后回滚）' ) =="
  ssh_run "
    TMP=\$(mktemp /tmp/qa_cleanup_XXXXXX.sql)
    cat > \"\$TMP\"
    PW=\$(docker inspect ${container} --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^POSTGRES_PASSWORD=//p')
    docker exec -i -e PGPASSWORD=\"\$PW\" ${container} psql -h 127.0.0.1 -p ${port} -U ${dbuser} -d ${dbname} -f - < \"\$TMP\"
    rc=\$?
    rm -f \"\$TMP\"
    exit \$rc
  " < "$sql"
  return $?
}

capture_sql() { # capture_sql <container> <db> <port> <user> <local_sql>：只回未对齐元组，NOTICE 仍走 stderr
  local container="$1" dbname="$2" port="$3" dbuser="$4" sql="$5"
  ssh_run "
    TMP=\$(mktemp /tmp/qa_cleanup_XXXXXX.sql)
    cat > \"\$TMP\"
    PW=\$(docker inspect ${container} --format '{{range .Config.Env}}{{println .}}{{end}}' | sed -n 's/^POSTGRES_PASSWORD=//p')
    docker exec -i -e PGPASSWORD=\"\$PW\" ${container} psql -h 127.0.0.1 -p ${port} -U ${dbuser} -d ${dbname} -Atq -f - < \"\$TMP\"
    rc=\$?
    rm -f \"\$TMP\"
    exit \$rc
  " < "$sql"
  return $?
}

# 本地镜像 order_no：平台侧空串 tenant_code 那批模块单的唯一认领线索。
# 必须在任何删除之前读——用户相一提交，这条线索就随 module_orders 行一起没了。
collect_mirror_orders() {
  local sql out line bad=0 n=0 list=""
  sql=$(mktemp "${TMPDIR:-/tmp}/qa_cleanup_mirror_$$.XXXXXX.sql") || return 1
  cat > "$sql" <<TPL
\set ON_ERROR_STOP on
SELECT DISTINCT mo.order_no
  FROM module_orders mo
  JOIN tenants t ON t.id = mo.tenant_id
 WHERE t.code = '${TENANT_CODE}'
   AND mo.order_no IS NOT NULL AND mo.order_no <> ''
 ORDER BY 1;
TPL
  out=$(capture_sql "$USER_PG_CONTAINER" "$USER_PG_DB" "$USER_PG_PORT" "$USER_PG_USER" "$sql")
  bad=$?
  rm -f "$sql"
  if [ "$bad" != "0" ]; then
    echo "== 失败：读不到 user_db 的本地镜像 order_no（rc=${bad}）——平台相无法补认空串单，中止 ==" >&2
    return 1
  fi
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    # 只允许字母数字与 _-：越界的值绝不拼进 SQL（宁可放弃补认，也不给注入留口）
    case "$line" in
      *[!A-Za-z0-9_-]*)
        echo "== 失败：镜像 order_no 含意外字符，拒绝拼进 SQL：$line ==" >&2
        return 1 ;;
    esac
    n=$((n+1))
    if [ "$n" = "1" ]; then list="('$line')"; else list="$list,('$line')"; fi
  done <<< "$out"
  if [ "$n" -gt 500 ]; then
    echo "== 失败：镜像 order_no 有 $n 枚，超出单相可拼装架的上界（500）——请人工核对归属 ==" >&2
    return 1
  fi
  if [ "$n" = "0" ]; then
    echo "  [i] 本地镜像里没有本租户的订单行：平台相只按 tenant_code 认领"
  else
    echo "  [i] 本地镜像补认候选 $n 枚 order_no（平台相只接受其中 tenant_code 为空串的行）"
    EXTRA_VALUES="$list"
  fi
  return 0
}

rc=0
if [ "$PHASE" != "user" ]; then
  collect_mirror_orders || exit 1
fi

# 平台相在前：见文件头「跑的顺序」说明
if [ "$PHASE" = "all" ] || [ "$PHASE" = "platform" ]; then
  build_pf_sql | sed -e "s/${TOKEN_CODE}/${TENANT_CODE}/g" -e "s/${TOKEN_EXTRA}/${EXTRA_VALUES}/g" > "$PF_SQL"
  printf '%s\n' "$TAIL" >> "$PF_SQL"
  run_phase "platform_db" "$PF_PG_CONTAINER" "$PF_PG_DB" "$PF_PG_PORT" "$PF_PG_USER" "$PF_SQL" || rc=$?
fi

if [ "$rc" = "0" ] && { [ "$PHASE" = "all" ] || [ "$PHASE" = "user" ]; }; then
  build_user_sql \
    | sed -e "s/${TOKEN_CODE}/${TENANT_CODE}/g" -e "s/${TOKEN_RESERVED}/${RESERVED_IDS}/g" > "$USER_SQL"
  printf '%s\n' "$TAIL" >> "$USER_SQL"
  run_phase "user_db" "$USER_PG_CONTAINER" "$USER_PG_DB" "$USER_PG_PORT" "$USER_PG_USER" "$USER_SQL" || rc=$?
fi

if [ "$rc" = "0" ]; then
  echo "== 完成：租户 ${TENANT_CODE} $( [ "$COMMIT" = "1" ] && echo '已清理' || echo '试算通过（未提交，加 --commit 生效）' ) =="
else
  echo "== 失败：任一护栏或复扫未过，本脚本不落盘（rc=${rc}）==" >&2
fi
exit "$rc"

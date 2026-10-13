#!/usr/bin/env bash
# check-text-encoding.py（文本编码门，G7）的用例：全在 mktemp 出来的假 git 树里做，
# 不往真仓库写一个字节；最后一格才拿真仓库当正控制。
#
# 为什么要有这个文件：门自己 `--self-check` 只喂内存里的字节夹具，它证明的是
# 「检测函数有牙」；「门认得出中文文件名」「豁免按次数放行」「缺基线要退 2」
# 「门真的挂在 make audit 与 CI 上」这些**只有在真树 + 真 git 上才成立**的判据，
# self-check 一条都证不了。缺了常驻用例，下一次改这个门的正则就只能靠人肉复跑。
#
# 格子与各自守的东西（共 18 格，缺一格就是"改了一半"）：
#   T1  干净假树 ⇒ rc=0（正控制：门不误伤）
#   T2  注入 U+FFFD ⇒ rc=1，且红因**点名那个文件**
#   T3  注入 GBK 字节 ⇒ rc=1 NOT_UTF8（非 UTF-8 无豁免通道）
#   T4  注入双重编码字节 ⇒ rc=1 MOJIBAKE
#   T5  基线次数 == 现算次数 ⇒ rc=0（刻意用法放行）
#   T6  现算 > 基线 ⇒ rc=1（新增替身字符）
#   T7  现算 < 基线 ⇒ rc=1 STALE（修掉了还不划掉基线，逼着维护基线）
#   T8  缺基线文件 ⇒ rc=2（"没扫"与"扫了没问题"必须分得开）
#   T9  基线登记了一个不在扫描面上的文件 ⇒ rc=2
#   T10 基线登记的文件读不到 ⇒ rc=2（chmod 000；以 root 跑时该格前提不成立，显式 SKIP）
#   T11 扫描面为 0（.gitignore 写 `*` 的空树）⇒ rc=2
#   T12 --repo 指一个非 git 目录 ⇒ rc=2（git ls-files 失败＝门没能力列面，不是绿）
#   T13 中文名文件真在面上：往 `说明.md` 注入 U+FFFD 必须红
#       （不带 `-z` 的 git ls-files 会把非 ASCII 路径八进制转义 ⇒ 这格就是那条回归的钉子）
#   T14 未跟踪又未忽略的文件真在面上 ⇒ 红并点名
#   T15 含 NUL 的二进制被跳过：里面同时放非法 UTF-8 字节，若没跳过必然红
#   T16 门自己的源码在 `--self-check` 下退 0
#   T17 接线轴：Makefile 的 `audit:` 里真有这一步，CI 的 push 与 pull_request paths
#       里真有「脚本本体 + 基线」两行（缺一行就是"只改判据不重跑这道门"）
#   T18 真仓库正控制：`--repo .` 退 0，且自证计数「文本文件数」> 5000
#       （只印 rc=0 不印数额的门，正则坏掉时读数照样好看）
set -u

ROOT=$(cd "$(dirname "$0")/.." && pwd)
CHECKER="$ROOT/scripts/check-text-encoding.py"
WF="$ROOT/.github/workflows/user-server-ci.yml"
WORK=$(mktemp -d /tmp/text-encoding-test.XXXXXX)
trap 'rm -rf "$WORK"' EXIT
FAIL=0
TOTAL=0

ok()  { echo "  ✓ $1"; TOTAL=$((TOTAL + 1)); }
bad() { echo "  ✗ $1"; FAIL=$((FAIL + 1)); TOTAL=$((TOTAL + 1)); }

# 红一定要能读：bad 的第二参数把门实际打出的那几行带出来，
# 只印"这格没过"的坏用例会把红因吞掉（本仓的门用例有过这一课）。
need_rc() {  # need_rc <期望码> <格名> <红因关键字|__ANY__> <命令...>
  local want="$1" name="$2" needle="$3"; shift 3
  local out got
  out=$("$@" 2>&1); got=$?
  if [ "$got" != "$want" ]; then
    bad "${name}：期望 rc=${want}，实得 rc=${got}"
    printf '%s\n' "$out" | sed 's/^/      │ /' | tail -4
    return
  fi
  if [ "$needle" != "__ANY__" ]; then
    if ! printf '%s\n' "$out" | grep -q -- "$needle"; then
      bad "${name}：rc 对上了但红因里没有「${needle}」"
      printf '%s\n' "$out" | sed 's/^/      │ /' | tail -6
      return
    fi
  fi
  ok "$name"
}

# 字节夹具：U+FFFD = \357\277\275；GBK「中文」= \326\320\327\306（对 UTF-8 非法）；
# 双重编码 = "中文" 的 UTF-8 字节被当 Latin-1 读走再存 UTF-8；NUL = \000
FFFD=$'\357\277\275'
GBK=$'\326\320\327\306'
MOJI=$'\303\244\302\270\302\255\303\246\302\226\302\207'

[ -f "$CHECKER" ] || { echo "FATAL: 找不到 $CHECKER"; exit 1; }

# ---- 公共夹具：一个真 git 树，两行干净中文，基线默认不登记任何东西 ----------------
newtree() {  # newtree <目录>
  local d="$1"
  mkdir -p "$d/scripts" "$d/docs"
  printf '第一行中文注释\n第二行也是中文\n' > "$d/docs/a.txt"
  ( cd "$d" && git -c user.email=t@t -c user.name=t -c commit.gpgsign=false \
        init -q . && git add -A >/dev/null && \
    git -c user.email=t@t -c user.name=t -c commit.gpgsign=false commit -qm seed )
  # 夹具的 git 前提必须真成立，否则每一格量的都不是同一件事（静默的假绿）。
  if ! git -C "$d" rev-parse HEAD >/dev/null 2>&1; then
    echo "FATAL: 夹具树 $d 没能建成 git 仓库（提交失败？全局 hooksPath / gpg 配置拦的？）"
    exit 1
  fi
  : > "$d/scripts/text-encoding.baseline"
  echo '# 占位基线（只有注释）' >> "$d/scripts/text-encoding.baseline"
}

T=$WORK/t1; newtree "$T"

# T1 干净树绿
need_rc 0 T1 __ANY__ python3 "$CHECKER" --repo "$T"

# T2..T7 都往 docs/a.txt 上注，注完立刻还原（本用例不动真仓库，临时树随 EXIT 删）
printf '坏了一个字 %s 后面照旧\n' "$FFFD" > "$T/docs/a.txt"
need_rc 1 T2 "docs/a.txt" python3 "$CHECKER" --repo "$T"

printf 'GBK 整段 %s\n' "$GBK" > "$T/docs/a.txt"
need_rc 1 T3 "NOT_UTF8" python3 "$CHECKER" --repo "$T"

printf '双重编码 %s\n' "$MOJI" > "$T/docs/a.txt"
need_rc 1 T4 "MOJIBAKE" python3 "$CHECKER" --repo "$T"

# T5 一次 U+FFFD + 基线登记 1 次 ⇒ 放行
printf '坏了一个字 %s 后面照旧\n' "$FFFD" > "$T/docs/a.txt"
printf 'docs/a.txt\t1\n' > "$T/scripts/text-encoding.baseline"
need_rc 0 T5 __ANY__ python3 "$CHECKER" --repo "$T"

# T6 现算 2 > 基线 1 ⇒ 红
printf '坏两个字 %s%s\n' "$FFFD" "$FFFD" > "$T/docs/a.txt"
need_rc 1 T6 "超出基线" python3 "$CHECKER" --repo "$T"

# T7 现算 1 < 基线 3 ⇒ STALE 红
printf '坏了一个字 %s 后面照旧\n' "$FFFD" > "$T/docs/a.txt"
printf 'docs/a.txt\t3\n' > "$T/scripts/text-encoding.baseline"
need_rc 1 T7 "STALE" python3 "$CHECKER" --repo "$T"

# T8 缺基线
printf '干净的\n' > "$T/docs/a.txt"; rm -f "$T/scripts/text-encoding.baseline"
need_rc 2 T8 "缺基线" python3 "$CHECKER" --repo "$T"

# T9 基线登记了面上没有的文件
printf 'docs/ghost.txt\t1\n' > "$T/scripts/text-encoding.baseline"
need_rc 2 T9 "不在扫描面" python3 "$CHECKER" --repo "$T"

# T10 登记的文件在面上但读不到（root 下 chmod 000 挡不住，前提不成立就显式 SKIP）
printf '坏了一个字 %s\n' "$FFFD" > "$T/docs/ghost.txt"
git -C "$T" add -A >/dev/null 2>&1
git -C "$T" -c user.email=t@t -c user.name=t -c commit.gpgsign=false commit -qm ghost >/dev/null 2>&1
chmod 000 "$T/docs/ghost.txt"
if head -c1 "$T/docs/ghost.txt" >/dev/null 2>&1; then
  ok "T10 SKIP：本进程读得开 chmod 000 的文件（root），这格前提不成立"
else
  need_rc 2 T10 "读不到" python3 "$CHECKER" --repo "$T"
fi
chmod 644 "$T/docs/ghost.txt"

# T11 扫描面为 0：.gitignore 写 `*`（它把自己也忽略掉，所以面上一个文件都不剩），
# 且这棵树**不提交任何东西**——一旦把 .gitignore 变成跟踪件，面就是 1 而不是 0，
# 这格会去撞另一条判据（rc=0 的"干净"），量的就不是空面了。
U=$WORK/t11; mkdir -p "$U/scripts"; printf '*\n' > "$U/.gitignore"
( cd "$U" && git -c user.email=t@t -c user.name=t -c commit.gpgsign=false init -q . )
printf '# 空面时这条豁免不该被当成"没违规"\n' > "$U/scripts/text-encoding.baseline"
need_rc 2 T11 "扫描面为 0" python3 "$CHECKER" --repo "$U"

# T12 --repo 指非 git 目录
V=$WORK/notagit; mkdir -p "$V/scripts"; printf 'x\n' > "$V/scripts/text-encoding.baseline"
need_rc 2 T12 "git ls-files 失败" python3 "$CHECKER" --repo "$V"

# T13 中文名文件真在面上（`-z` 那口井的钉子）
W=$WORK/t13; newtree "$W"; printf '菜单说明\n' > "$W/说明.md"
git -C "$W" add -A >/dev/null 2>&1
git -C "$W" -c user.email=t@t -c user.name=t -c commit.gpgsign=false commit -qm cn >/dev/null 2>&1
need_rc 0 T13a __ANY__ python3 "$CHECKER" --repo "$W"
printf '坏了一个字 %s\n' "$FFFD" > "$W/说明.md"
need_rc 1 T13b "说明.md" python3 "$CHECKER" --repo "$W"

# T14 未跟踪又未忽略的文件在面上
printf '坏了一个字 %s\n' "$FFFD" > "$W/loose.txt"
need_rc 1 T14 "loose.txt" python3 "$CHECKER" --repo "$W"

# T15 含 NUL 的二进制被跳过：同时塞非法 UTF-8，没跳过就一定红
printf 'BIN\000%s' "$GBK" > "$W/blob.bin"
rm -f "$W/说明.md" "$W/loose.txt"          # 只留这一处可疑内容，隔离判据
git -C "$W" add -A >/dev/null 2>&1
git -C "$W" -c user.email=t@t -c user.name=t -c commit.gpgsign=false commit -qm bin >/dev/null 2>&1
printf '菜单说明\n' > "$W/说明.md"
need_rc 0 T15 __ANY__ python3 "$CHECKER" --repo "$W"

# T16 门自己的内存自检
need_rc 0 T16 "自检" python3 "$CHECKER" --self-check

# T17 接线轴：Makefile 与 CI 两个事件 paths 都真接着
if awk '/^audit:/,/^$/' "$ROOT/Makefile" | grep -q 'scripts/check-text-encoding.py'; then
  ok "T17a make audit 里有这一步"
else
  bad "T17a Makefile 的 audit 目标里没有 check-text-encoding.py（门被摘掉了）"
fi
push_n=$(awk '/^  push:/,/^  pull_request:/' "$WF" | grep -c "scripts/check-text-encoding.py")
pr_n=$(awk '/^  pull_request:/,/^# ===/' "$WF" | grep -c "scripts/check-text-encoding.py")
base_push=$(awk '/^  push:/,/^  pull_request:/' "$WF" | grep -c "scripts/text-encoding.baseline")
base_pr=$(awk '/^  pull_request:/,/^# ===/' "$WF" | grep -c "scripts/text-encoding.baseline")
if [ "$push_n" -ge 1 ] && [ "$pr_n" -ge 1 ] && [ "$base_push" -ge 1 ] && [ "$base_pr" -ge 1 ]; then
  ok "T17b CI 两个事件各含脚本与基线（push ${push_n}/${base_push}、pull_request ${pr_n}/${base_pr}）"
else
  bad "T17b CI paths 缺行：push 脚本 ${push_n} 基线 ${base_push}；pull_request 脚本 ${pr_n} 基线 ${base_pr}"
fi
step_n=$(grep -c 'run: python3 scripts/check-text-encoding.py' "$WF")
if [ "$step_n" -ge 1 ]; then
  ok "T17c CI 里真有一步在跑它（${step_n} 处）"
else
  bad "T17c CI 里没有执行它的那一步 ⇒ paths 只是空登记"
fi

# T18 真仓库正控制：计数必须落在一个可信的量级上，不然"绿"没有含义
live_out=$(python3 "$CHECKER" --repo "$ROOT" 2>&1); live_rc=$?
live_n=$(printf '%s\n' "$live_out" | sed -n 's/.*文本 \([0-9][0-9]*\) 个.*/\1/p' | head -1)
if [ "$live_rc" = 0 ] && [ -n "$live_n" ] && [ "$live_n" -gt 5000 ]; then
  ok "T18 真仓库绿，文本 ${live_n} 个"
else
  bad "T18 真仓库 rc=${live_rc}、文本计数=「${live_n:-未打印}」（应 > 5000）"
  printf '%s\n' "$live_out" | sed 's/^/      │ /' | tail -4
fi

echo "断言处数=${TOTAL} 失败=$FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "❌ check-text-encoding 用例未通过"
  exit 1
fi
echo "✅ check-text-encoding 用例通过"

#!/usr/bin/env bash
# =============================================================================
# bug-verify.sh —— spec-kit bug 三段流之后的确定性验证阶梯
#
# 为什么要有它：修完一个 bug 之后"必须发生的动作"是固定的，但每次现场手敲就会漂 ——
# 漏跑一次 make audit、把别的泳道的红当成自己的、报告里的读数其实测在另一个 commit
# 上。这里把这套动作钉成一条命令，并且**默认只认已提交的字节**：判定跑在
# `git clone --shared` 出来的干净树上，那棵树里没有工作区的未提交文件，于是
# "忘记 git add 某个定义文件"这类错会以编译红的形式当场出现，而不是等别人克隆才炸。
#
# 用法：
#   bash scripts/bug-verify.sh --slug <bug-slug> [选项]
#
# 选项：
#   --packages "p1 p2"   受影响 Go 包（相对 user-server/），跑 go test；默认不跑
#   --race               go test 加 -race（只在给了 --packages 时有意义）
#   --allow-dirty PATH   把这些路径的工作树版本带进克隆（用于"还没提交就先验一次"；
#                        必须逐个点名，绝不从 git status 取清单）
#   --skip-audit         只跑 build/vet（make audit 整链较慢时用来快速自处）
#   --lint-md            在克隆里跑 markdownlint-cli2（make audit 里没有它，唯一执行点是
#                        CI 的 Markdown Lint 作业 ⇒ 不带这一档，`**/*.md` 的形状违规会
#                        一路活到 CI 才红）
#   --push-plan          额外打印两双远端相对 HEAD 的领先/落后（只读，不推）
#   --keep               保留克隆目录并打印路径（默认退出时删掉，取证日志保留）
#
# 退出码：
#   0 = 该跑的都绿
#   1 = 未通过（编译 / vet / 静态门 / 用例真失败，或 --push-plan 查出远端非 fast-forward）
#   2 = 前提不满足（三段产物缺失、磁盘不足、无测试库口令、不在 git 仓内、克隆不认账）
#       —— 2 不是"通过"，也不允许被读成"跑过了"
#
# 取证日志：写在 ${TMPDIR}/bug-verify/<slug>-<时间戳>/ 下，每步一份，复跑不覆盖上一轮。
# 报告里引用读数时要点名日志目录与 MEASURED_COMMIT 那一行。
#
# 兼容 bash 3.2（macOS 自带）：不使用关联数组；变量紧跟中文一律写成 ${VAR}。
# =============================================================================

set -uo pipefail

RED=''; GREEN=''; YELLOW=''; NC=''
if [[ -t 1 ]]; then
    RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; YELLOW=$'\033[1;33m'; NC=$'\033[0m'
fi

STEP_RC=0        # 内容红
PRECOND_RC=0     # 前提不满足
FAILURES=''

ok()   { echo "${GREEN}✅ ${1}${NC}"; }
warn() { echo "${YELLOW}⚠️  ${1}${NC}"; }
bad()  {
    echo "${RED}❌ ${1}${NC}"
    if [[ -n "${2:-}" ]]; then
        echo "${RED}   红因: ${2}${NC}"
    fi
}

usage() {
    cat <<'TXT'
用法: bash scripts/bug-verify.sh --slug <bug-slug> [选项]
  --packages "p1 p2"   受影响 Go 包（相对 user-server/）跑 go test；默认不跑
  --race               go test 加 -race
  --allow-dirty PATH   把这些路径的工作树版本带进克隆（逐个点名）
  --skip-audit         只跑 build/vet，不跑 make -k audit
  --lint-md            在克隆里跑 markdownlint-cli2（make audit 里没有它，唯一执行点是
                       CI 的 Markdown Lint 作业 ⇒ 不带这一档，**/*.md 的形状违规会
                       一路活到 CI 才红）
  --push-plan          额外检查 gitee-upstream / upstream 的 fast-forward（只读）
  --keep               保留干净克隆目录并打印路径
退出码: 0 全绿 / 1 未通过（判定红或不可直推） / 2 前提不满足（不是通过）
TXT
    exit 2
}

SLUG=''
PACKAGES=''
RACE=0
SKIP_AUDIT=0
LINT_MD=0
PUSH_PLAN=0
KEEP=0
DIRTY_LIST=()

# run_step 拼红因 grep 时用的可选前缀模式。默认必须「永不匹配」：空串会让那个
# 交替表达式变成 `|undefined:|...`，开头的空分支匹配一切，于是任何一档红了都把
# 日志头三行当红因印出来。
EXTRA_REASON_RE='ZZZ_NEVER_MATCH_ZZZ'

while [[ $# -gt 0 ]]; do
    case "$1" in
        --slug)       SLUG="${2:-}"; shift 2 ;;
        --packages)   PACKAGES="${2:-}"; shift 2 ;;
        --race)       RACE=1; shift ;;
        --skip-audit) SKIP_AUDIT=1; shift ;;
        --lint-md)    LINT_MD=1; shift ;;
        --push-plan)  PUSH_PLAN=1; shift ;;
        --keep)       KEEP=1; shift ;;
        --allow-dirty)
            shift
            while [[ $# -gt 0 && "${1:0:2}" != '--' ]]; do
                DIRTY_LIST+=("$1"); shift
            done
            ;;
        -h|--help)    usage ;;
        *)            echo "未知参数: $1" >&2; usage ;;
    esac
done

if [[ -z "${SLUG}" ]]; then
    bad "必须给 --slug <bug-slug>" "三段产物在 .specify/bugs/<slug>/，没有 slug 无从对账"
    usage
fi

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -z "${REPO_ROOT}" ]]; then
    bad "当前目录不在 git 仓内，无法确定项目根" "本脚本须在 hivemtk 仓的任一子目录里运行"
    exit 2
fi

MEASURED_COMMIT="$(git -C "${REPO_ROOT}" rev-parse HEAD)"
BRANCH="$(git -C "${REPO_ROOT}" rev-parse --abbrev-ref HEAD)"
TS="$(date -u +%Y%m%d-%H%M%S)"
EVID="${TMPDIR:-/tmp}/bug-verify/${SLUG}-${TS}"
mkdir -p "${EVID}"

echo "═══════════════════════════════════════════════════════════════════════"
echo " bug-verify  slug=${SLUG}"
echo " 分支      ${BRANCH}   HEAD=${MEASURED_COMMIT}"
echo " 取证日志  ${EVID}"
echo "═══════════════════════════════════════════════════════════════════════"

# -----------------------------------------------------------------------------
# 1/5 三段产物对账：assess → fix → test 是不是都真落了笔
# -----------------------------------------------------------------------------
echo
echo "── 1/5 三段产物（.specify/bugs/${SLUG}/）──"
BUG_DIR="${REPO_ROOT}/.specify/bugs/${SLUG}"
if [[ ! -d "${BUG_DIR}" ]]; then
    bad "找不到 bug 目录 ${BUG_DIR}" "先跑 /speckit-bug-assess slug=${SLUG}"
    PRECOND_RC=2
else
    # 每份产物必须带它那一条定稿状态行；缺行＝那一阶段没写完（或写完没提交）
    for spec in "assessment.md|- **Verdict**" "fix.md|- **Status**" "test.md|- **Result**"; do
        fname="${spec%%|*}"
        needle="${spec#*|}"
        fpath="${BUG_DIR}/${fname}"
        if [[ ! -f "${fpath}" ]]; then
            bad "${fname} 不存在" "三段流有一阶段未落笔：${fpath}"
            PRECOND_RC=2
            continue
        fi
        # 三条状态行都以 `- ` 开头，grep 会把它们当选项吃掉（实测退 2 打成「缺状态行」假红）
        line="$(grep -m1 -F -- "${needle}" "${fpath}" || true)"
        if [[ -z "${line}" ]]; then
            bad "${fname} 缺状态行 ${needle}" "文件在，但阶段没定稿；不许用「写了一半」的产物收口"
            PRECOND_RC=2
        else
            ok "$(printf '%-14s %s' "${fname}" "${line}")"
        fi
        # 干净树判定看不见未跟踪的产物：文档里的链接在活树可解析、在克隆里就是死链
        if git -C "${REPO_ROOT}" ls-files --error-unmatch "${BUG_DIR#${REPO_ROOT}/}/${fname}" >/dev/null 2>&1; then
            echo "     跟踪状态: 在 HEAD 里"
        else
            warn "     跟踪状态: 未跟踪 —— 克隆里的判定看不见它（下一步会按 HEAD 的字节跑）"
        fi
    done
fi

# -----------------------------------------------------------------------------
# 2/5 环境预检：这三样不齐会造出假红/假绿，先拦下来
# -----------------------------------------------------------------------------
echo
echo "── 2/5 环境预检 ──"
AVAIL_KB="$(df -kP "${REPO_ROOT}" | awk 'NR==2{print $4}')"
MIN_KB=$((3 * 1024 * 1024))   # 3 GiB：一次 go build ./... 加一门 make audit 的量级
if [[ -z "${AVAIL_KB}" ]]; then
    bad "读不到 ${REPO_ROOT} 所在卷的可用空间" "df 没给出第二行；宁停不猜"
    PRECOND_RC=2
elif [[ "${AVAIL_KB}" -lt "${MIN_KB}" ]]; then
    bad "可用磁盘 $((AVAIL_KB / 1024)) MiB < 3 GiB" "盘满会让 go vet 绿而 go build 红；先腾空间（scripts/trim-go-cache.sh）"
    PRECOND_RC=2
else
    ok "磁盘剩余 $((AVAIL_KB / 1024)) MiB"
fi

if command -v go >/dev/null 2>&1; then
    ok "go $(go env GOVERSION 2>/dev/null || go version | awk '{print $3}')"
else
    bad "PATH 上没有 go" "build/vet/test 三档都无从跑起，本条结论只能是「没跑」"
    PRECOND_RC=2
fi

if command -v python3 >/dev/null 2>&1; then
    # /usr/bin/python3 是 Xcode 许可 shim：装了但未同意许可时退 69，make audit 里
    # 十几枚 python 门会一起红，红形和「仓库有缺陷」长得一模一样。
    if python3 -c 'pass' >/dev/null 2>&1; then
        ok "python3 可用（$(python3 -V 2>&1)）"
    else
        rc_py=0
        python3 -c 'pass' >/dev/null 2>&1 || rc_py=$?
        bad "python3 存在但跑不动（rc=${rc_py}）" "69＝Xcode 许可未同意；make audit 需要它，属机器前提不属仓库"
        PRECOND_RC=2
    fi
fi

if [[ -n "${PACKAGES}" ]]; then
    if [[ -z "${POSTGRES_TEST_PASSWORD:-}" ]]; then
        # 测试侧读的是 POSTGRES_TEST_PASSWORD，值来自 .env 的 POSTGRES_PASSWORD
        # （口径同 user-server/internal/pkg/testutil/testdb.go 的用法注释与常驻电池脚本）。
        # 只取第一个键（重复键时那才是实际生效的一份），且永不回显取值。
        pw="$(awk -F'= *' '/^POSTGRES_PASSWORD=/{print $2; exit}' "${REPO_ROOT}/.env" 2>/dev/null || true)"
        if [[ -n "${pw}" ]]; then
            export POSTGRES_TEST_PASSWORD="${pw}"
            ok "POSTGRES_TEST_PASSWORD 已从 .env 的 POSTGRES_PASSWORD 注入（长度 ${#pw}，值不回显）"
        elif [[ -n "${POSTGRES_PASSWORD:-}" ]]; then
            ok "沿用环境里的 POSTGRES_PASSWORD（testutil 会拿它兜底）"
        else
            bad "既没有 POSTGRES_TEST_PASSWORD / POSTGRES_PASSWORD，.env 里也取不到" "带 --packages 的这一档会是 ENV-BROKEN；注意用例连不上库时是 Skip 不是 Fail，Skip 满屏不等于绿"
            PRECOND_RC=2
        fi
    else
        ok "POSTGRES_TEST_PASSWORD 已由环境提供（不回显）"
    fi
    # CI 全局注入的开关；本地缺它时两包 Fatalf、两包 Skip —— 那是环境形态不是缺陷
    export EMBEDDING_ALLOW_FALLBACK="${EMBEDDING_ALLOW_FALLBACK:-true}"
    ok "EMBEDDING_ALLOW_FALLBACK=${EMBEDDING_ALLOW_FALLBACK}"

    # 库不可达时 testutil 走 Skipf 而不是 Fail —— "0 failed" 会是全场跳过的假绿，
    # 所以这一档开工前先把端口探一遍（探测口径同 testdb.go 的候选端口）。
    db_port=''
    for cand in 5432 8202 8232; do
        if (exec 3<>/dev/tcp/127.0.0.1/"${cand}") 2>/dev/null; then
            db_port="${cand}"
            break
        fi
    done
    if [[ -n "${db_port}" ]]; then
        ok "PostgreSQL 可达（127.0.0.1:${db_port}）"
    else
        bad "127.0.0.1 的 5432/8202/8232 都不通" "连不上库时用例是 Skip 不是 Fail；带 --packages 的这一档只会得到「跳过式假绿」，判废"
        PRECOND_RC=2
    fi
fi

if [[ "${PRECOND_RC}" -eq 2 ]]; then
    echo
    bad "前提不满足，未开始判定" "退出码 2；修好前提再重跑，别把这一条读成绿"
    exit 2
fi

# -----------------------------------------------------------------------------
# 3/5 建干净树：只含已提交字节的 --shared 克隆
# -----------------------------------------------------------------------------
echo
echo "── 3/5 干净克隆（只含 HEAD 的字节）──"
WORK_PARENT="$(mktemp -d "${TMPDIR:-/tmp}/bug-verify-tree.XXXXXX")"
WORK="${WORK_PARENT}/hivemtk"   # 门脚本按仓名推根，克隆必须摆成 <某目录>/hivemtk
# 中途以 exit 2 退出（判定废掉）时也要把克隆收掉，一次干净克隆是几百 MB 的对象链接树
cleanup_work() { [[ "${KEEP}" -eq 1 ]] || rm -rf "${WORK_PARENT}"; }
trap cleanup_work EXIT
if ! git clone --shared --quiet "${REPO_ROOT}" "${WORK}" >"${EVID}/00-clone.log" 2>&1; then
    bad "git clone --shared 失败" "见 ${EVID}/00-clone.log"
    cat "${EVID}/00-clone.log"
    exit 2
fi
CLONE_HEAD="$(git -C "${WORK}" rev-parse HEAD)"
if [[ "${CLONE_HEAD}" != "${MEASURED_COMMIT}" ]]; then
    bad "克隆 HEAD 与来源 HEAD 不一致" "克隆=${CLONE_HEAD} 来源=${MEASURED_COMMIT}；读数属于哪个 commit 说不清，判废"
    exit 2
fi
ok "克隆 HEAD = ${MEASURED_COMMIT}（与来源同值）"

if [[ ${#DIRTY_LIST[@]} -gt 0 ]]; then
    # 显式点名才带：把本泳道未提交的改动做成 patch 应用进克隆，未跟踪的那份单独 cp。
    patch="${EVID}/00-lane.patch"
    git -C "${REPO_ROOT}" -c core.quotepath=off diff HEAD -- "${DIRTY_LIST[@]}" >"${patch}"
    applied=0
    if [[ -s "${patch}" ]]; then
        if git -C "${WORK}" apply "${patch}" >"${EVID}/00-apply.log" 2>&1; then
            applied=1
        else
            bad "工作树改动应用进克隆失败" "见 ${EVID}/00-apply.log（多半是同一文件正被并行泳道改）"
            cat "${EVID}/00-apply.log"
            exit 2
        fi
    fi
    copied=0
    for p in "${DIRTY_LIST[@]}"; do
        if ! git -C "${REPO_ROOT}" ls-files --error-unmatch "${p}" >/dev/null 2>&1 && [[ -f "${REPO_ROOT}/${p}" ]]; then
            mkdir -p "${WORK}/$(dirname "${p}")"
            cp -p "${REPO_ROOT}/${p}" "${WORK}/${p}"
            copied=$((copied + 1))
        fi
    done
    # 克隆里只许列出名单内的路径，否则本轮判定混进了别人泳道的字节
    git -C "${WORK}" -c core.quotepath=off status --short >"${EVID}/00-clone-status.log"
    unexpected=''
    while read -r st f; do
        [[ -z "${f}" ]] && continue
        found=0
        for p in "${DIRTY_LIST[@]}"; do
            [[ "${f}" == "${p}" ]] && found=1
        done
        [[ "${found}" -eq 0 ]] && unexpected="${unexpected}${f} "
    done <"${EVID}/00-clone-status.log"
    if [[ -n "${unexpected}" ]]; then
        bad "克隆里出现了 --allow-dirty 名单之外的改动" "这些不该进本轮判定: ${unexpected}"
        exit 2
    fi
    warn "--allow-dirty 生效：patch 应用=${applied} 未跟踪复制=${copied}（这一档读数测的是「HEAD + 点名路径」，不是纯 HEAD）"
fi

# -----------------------------------------------------------------------------
# 4/5 判定：build → vet →（make -k audit）→（go test）
# -----------------------------------------------------------------------------
echo
echo "── 4/5 判定（全部在干净树里）──"
run_step() {
    # $1 步骤名  $2 日志名  $3 工作目录，其余为命令
    local name="$1" logname="$2" wd="$3"
    shift 3
    local log="${EVID}/${logname}"
    local rc=0
    (cd "${wd}" && "$@") >"${log}" 2>&1 || rc=$?
    if [[ "${rc}" -eq 0 ]]; then
        ok "${name} rc=0"
    else
        # 红因必须当场读出来：只印 rc 等于让人回去翻，而翻的时候往往已经在猜
        local reason
        reason="$(grep -m3 -E -- "${EXTRA_REASON_RE}|undefined:|cannot find|Error:|FAIL|panic:|\[ERROR\]" "${log}" | tr '\n' ';')"
        bad "${name} rc=${rc}" "${reason:-见 ${log}}"
        echo "   日志: ${log}"
        STEP_RC=1
        FAILURES="${FAILURES}${name} "
    fi
}

run_step "go build ./..." 10-build.log "${WORK}/user-server" go build ./...
run_step "go vet ./..."   11-vet.log   "${WORK}/user-server" go vet ./...

if [[ "${SKIP_AUDIT}" -eq 1 ]]; then
    warn "跳过 make audit（--skip-audit）：静态门这一档＝没读，报告里不许写成「全套门已过」"
else
    run_step "make -k audit" 12-audit.log "${WORK}" make -k audit
fi

# markdownlint 在 make audit 里没有档，唯一执行点是 CI 的 Markdown Lint 作业。
# 不带 --lint-md 的话，**/*.md 的形状违规会一路活到 CI 才红，所以这档必须能显式点名。
if [[ "${LINT_MD}" -eq 1 ]]; then
    # markdownlint 的红因行形如 `MD047/single-trailing-newline`，上面那组通用模式一个都匹配不到，
    # 不补这个变量的话这一档红了只会印「见 <log>」，违背本脚本「红因当场读出来」的约定。
    EXTRA_REASON_RE='MD[0-9]{2}/'
    run_step "markdownlint-cli2 (--lint-md)" 14-markdownlint.log "${WORK}" npx -y markdownlint-cli2
    EXTRA_REASON_RE='ZZZ_NEVER_MATCH_ZZZ'
fi

if [[ -n "${PACKAGES}" ]]; then
    # 包名列表就是要按词展开
    # shellcheck disable=SC2086
    set -- -count=1 ${PACKAGES}
    if [[ "${RACE}" -eq 1 ]]; then
        set -- -count=1 -race ${PACKAGES}
    fi
    run_step "go test ${PACKAGES}" 13-test.log "${WORK}/user-server" go test "${@}"
    # 不带 -test.v 时逐用例的 PASS 行根本不印（计数恒 0 是量错了对象），这里只报能真报的
    echo "   包级: ok=$(grep -cE '^ok( |\$)' "${EVID}/13-test.log") FAIL=$(grep -cE '^FAIL' "${EVID}/13-test.log")  用例级: FAIL=$(grep -c -- '--- FAIL' "${EVID}/13-test.log") SKIP=$(grep -c -- '--- SKIP' "${EVID}/13-test.log")"
fi

# -----------------------------------------------------------------------------
# 5/5 推送前提（只读）：规则0 要求 fetch 后确认 fast-forward
# -----------------------------------------------------------------------------
if [[ "${PUSH_PLAN}" -eq 1 ]]; then
    echo
    echo "── 5/5 推送前提（只读，不推）──"
    for remote in gitee-upstream upstream; do
        if ! git -C "${REPO_ROOT}" remote get-url "${remote}" >/dev/null 2>&1; then
            warn "${remote} 不在本仓 remote 里"
            continue
        fi
        if ! git -C "${REPO_ROOT}" fetch --quiet "${remote}" >"${EVID}/20-fetch-${remote}.log" 2>&1; then
            warn "fetch ${remote} 失败" "见 ${EVID}/20-fetch-${remote}.log；这一条远端的 FF 判定无效"
            continue
        fi
        counts="$(git -C "${REPO_ROOT}" rev-list --left-right --count "${remote}/master...${BRANCH}")"
        behind="$(echo "${counts}" | awk '{print $1}')"
        ahead="$(echo "${counts}" | awk '{print $2}')"
        tip="$(git -C "${REPO_ROOT}" rev-parse --short "${remote}/master")"
        if [[ "${behind}" -eq 0 ]]; then
            ok "${remote}: 本地领先 ${ahead} 笔，远端无独有提交 ⇒ 可 fast-forward（远端 tip=${tip}）"
        else
            bad "${remote}: 远端独有 ${behind} 笔" "不可直推；先在干净树里 merge ${remote}/master 再推（规则0）"
            STEP_RC=1
            FAILURES="${FAILURES}${remote}-non-ff "
        fi
    done
fi

# -----------------------------------------------------------------------------
# 结论
# -----------------------------------------------------------------------------
echo
echo "═══════════════════════════════════════════════════════════════════════"
echo " MEASURED_COMMIT ${MEASURED_COMMIT}"
echo " EVIDENCE        ${EVID}"
if [[ "${STEP_RC}" -eq 0 ]]; then
    ok "全绿（前提齐、干净树判定通过）"
    rc_out=0
else
    # 逐项点名失败格：把「用例红」和「不可直推」混成一句"失败了"会让人去翻错的地方
    bad "未通过：${FAILURES}" "按规则2 当轮修掉再重跑；报告里不许把这写成已过"
    rc_out=1
fi
if [[ "${KEEP}" -eq 1 ]]; then
    echo " 克隆保留在 ${WORK}（--keep）"
fi
echo "═══════════════════════════════════════════════════════════════════════"
exit "${rc_out}"

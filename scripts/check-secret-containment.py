#!/usr/bin/env python3
"""真值包含闸：不问"形状像不像口令"，直接问"这台机器上那几枚私有口令/密钥字节，有没有出现在要推的字节里"。

候选集 = hub `.env` 里键名命中 PASS|SECRET|TOKEN|KEY|DSN|CRED|SALT 且值长≥6 的取值
        ∪ 容器 `mtk-postgres` 的 env 里同类键的取值（同名同值并枚）
面 = 树内取证产物 *.log ／ 本笔提交全量差异 ／ 审计文档 ／ 探针脚本 ／ 整棵 tip 树（git grep）
   （加 `--transcripts` 再看第 6 个面：本机会话转写。它只报告、不进判定，理由见 transcripts_report 上方）
一枚值若已在排除取证档后的已跟踪源码里出现（弱口令表、测试常量），单列为"公开常量"，不计入口令面。
反向自证：把候选里第一枚临时拼进一行假日志，出现数必须 +1，否则本闸没牙。
值本身任何形态都不打印：RAW 命中只印 键名／值长／各面计数。
候选取不到 ⇒ 退 2（ENV-BROKEN：没跑过，不是绿）。任一私有值出现 ⇒ 退 1。全清 ⇒ 退 0。
`.env` 的坐标＝现取的仓根（`git rev-parse --show-toplevel`），`HUB_ENV_FILE` 可覆盖；
上面那三个判据文件任一不在原位（＝cwd 不是仓根）同样退 2，退 1 只留给真的发现了明文。
执行入口：本机 `.githooks/pre-push`（与 `check-secrets.sh` 在同一道钩子里串跑）；**不进 CI**，
因为候选源是本机 `.env` 与活容器 —— CI 上跑只会得到一扇恒退 2 的哑门。
"""
import os
import re
import subprocess
import sys
import time
from pathlib import Path

SECRET_KEY_RE = re.compile(r"(PASS|SECRET|TOKEN|KEY|DSN|CRED|SALT)", re.I)
LOG_GLOB = "docs/superpowers/specs/ledger/logs/**/*.log"
DOC = "docs/architecture/CHANNEL_INTEGRATION_AUDIT_2026-09.md"
PROBE = "scripts/cap-ab-53300-probe.py"


def hub_env() -> Path:
    """.env 的坐标：现取仓根，`HUB_ENV_FILE` 可覆盖。

    原先写死本机绝对路径。换一台机器、或本仓被检出到别的目录时，`read_text()` 抛
    FileNotFoundError ⇒ 进程退 1，而 1 在本闸口径里是「发现明文」——
    「仓库不在我原来的位置」会被下游读成一次泄露。挂进 .githooks/pre-push 后
    这个混淆会直接变成别人的推送被莫名拦下，故改成可解析的坐标 + 缺席时退 2。
    """
    override = os.environ.get("HUB_ENV_FILE")
    if override:
        return Path(override)
    top = subprocess.run(["git", "rev-parse", "--show-toplevel"],
                         capture_output=True, text=True).stdout.strip()
    return (Path(top) if top else Path.cwd()) / ".env"


def env_values(path):
    out = []
    for line in path.read_text(errors="replace").splitlines():
        m = re.match(r"\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$", line)
        if not m or not SECRET_KEY_RE.search(m.group(1)):
            continue
        v = m.group(2).strip().strip('"').strip("'")
        if len(v) >= 6:
            out.append((m.group(1), v))
    return out


def container_values():
    try:
        raw = subprocess.run(["docker", "inspect", "mtk-postgres", "--format",
                              "{{range .Config.Env}}{{println .}}{{end}}"],
                             capture_output=True, text=True, timeout=30).stdout
    except Exception as e:
        print(f"[容器 env] 读取失败：{type(e).__name__}（本面缺席，候选只剩 .env）")
        return []
    out = []
    for line in raw.splitlines():
        if "=" not in line:
            continue
        k, v = line.split("=", 1)
        if SECRET_KEY_RE.search(k) and len(v) >= 6:
            out.append((k, v))
    return out


def count(haystacks, needle):
    return sum(h.count(needle) for h in haystacks)


def grep_count(args, value):
    r = subprocess.run(["git", "grep", "-c", "-F", "-e", value] + args,
                       capture_output=True, text=True)
    return sum(int(ln.rsplit(":", 1)[-1]) for ln in r.stdout.splitlines()
               if ln.rsplit(":", 1)[-1].isdigit())


TRANSCRIPT_GLOB = "projects/*/*.jsonl"
# 转写面为什么只报告、不进判定：① 活动会话那份**必然**含明文（此刻正在追加），
# 把它算进 rc 会让闸恒红；② 这一维的收敛手段是**轮换**（旧值当场作废），不是逐份改写
# ——改用户会话记录不可逆，且 §23.21 第 8 段⑨ 已把"取证链不许伪造"立为口径；
# ③ 面在本机，别人机器与 CI 上取不到同一批文件（与整闸不入 `make audit` 同判）。
# 要的是"这一维现在有多大、有没有随轮换收窄"的可复跑读数，不是又一扇门。
def transcripts_report(vals) -> None:
    root = Path.home() / ".qoder-cn"
    files = sorted(root.glob(TRANSCRIPT_GLOB))
    if not files:
        print(f"本机转写面：{root}/{TRANSCRIPT_GLOB} 零文件 ⇒ 这一维无从测（不是 0 命中）")
        return
    per_key, dirty, total = {}, [], 0
    for f in files:
        try:
            txt = f.read_text(errors="replace")
        except OSError as e:
            print(f"  读不过：{f.name[:8]}（{type(e).__name__}）⇒ 该份缺席，计数是下界")
            continue
        row = {}
        for k, v in vals:
            c = txt.count(v)
            if c:
                row[k] = c
                per_key[k] = per_key.get(k, 0) + c
                total += c
        if row:
            dirty.append((f, row))
    # 牙齿：转写面走的是同一套 `count`，所以自证＝临时文件里种一枚候选值，必须被数到。
    import tempfile
    with tempfile.TemporaryDirectory(prefix="sectcontain-") as td:
        seed = Path(td) / "seed.jsonl"
        seed.write_text(f'noise {vals[0][1]} noise', encoding="utf-8")
        teeth = count([seed.read_text(encoding="utf-8")], vals[0][1])
    mb = sum(f.stat().st_size for f in files) / 1e6
    print(f"本机转写面（只报告，不计入判定）：{len(files)} 份／{mb:.1f} MB／含现值 {len(dirty)} 份／命中 {total} 次")
    print(f"  反向自证：种入临时份被数到 {teeth} 次（须 1）⇒ {'转写面有牙' if teeth == 1 else '转写面失效，本行读数不可引'}")
    for k, c in sorted(per_key.items(), key=lambda x: -x[1]):
        print(f"  键 {k}：{c} 次")
    for f, row in sorted(dirty, key=lambda x: -sum(x[1].values())):
        import datetime
        mt = datetime.datetime.fromtimestamp(f.stat().st_mtime).strftime("%m-%d %H:%M")
        live = "（此刻仍在追加＝活动会话）" if time.time() - f.stat().st_mtime < 600 else ""
        print(f"  {f.name[:8]} mtime={mt} 命中={sum(row.values())} 键={sorted(row)}{live}")


def main() -> int:
    env_path = hub_env()
    # 三个要读的面先验存在。缺席时退 2（"没跑过"），而不是让 read_text() 抛异常退 1 ——
    # 1 在本闸口径里＝"发现了明文"，两种红长得一样，下游就会把"cwd 不在仓根"当成一次泄露。
    absent = [f"{label} {path}" for label, path in
              (("hub .env", env_path), ("审计文档", Path(DOC)), ("探针脚本", Path(PROBE)))
              if not path.is_file()]
    if absent:
        print("⇒ ENV-BROKEN：判据文件不在原位（多半是 cwd 不是仓根）：" + "；".join(absent))
        return 2
    by_value = {}
    for k, v in env_values(env_path) + container_values():
        by_value.setdefault(v, k)
    # 一律按 (键名, 值) 迭代：键名可打印，值绝不入档、不入屏
    vals = [(k, v) for v, k in by_value.items()]
    print(f"候选真值枚数={len(vals)}（hub .env ∪ 容器 mtk-postgres env 的口令类值，按值并枚）")
    if not vals:
        print("⇒ ENV-BROKEN：一枚都没取到，本闸没有可比对象，不许判绿")
        return 2

    logs = [p.read_text(errors="replace") for p in sorted(Path(".").glob(LOG_GLOB))]
    diff = subprocess.run(["git", "show", "HEAD", "--no-color"],
                          capture_output=True, text=True, check=True).stdout
    doc = Path(DOC).read_text()
    probe = Path(PROBE).read_text()
    print(f"面：树内产物 {len(logs)} 份／提交差异 {len(diff)} 字节／文档＋探针 2 份／整棵 tip 树")

    pool = logs + [diff, doc, probe]
    base = count(pool, vals[0][1])
    planted = count(pool + [f"noise password={vals[0][1]} noise"], vals[0][1])
    print(f"反向自证：未种={base} 种入={planted}（须 planted=base+1）⇒ {'有牙' if planted == base + 1 else '失效'}")
    if planted != base + 1:
        return 1

    total = public_total = 0
    for k, v in vals:
        # 反向自证过的一类故障：值与键名在字典里对调之后，屏上印的就是值。
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", k):
            print(f"⇒ ABORT：待打印的身份字段不是 env 键名形状（长度 {len(k)}），疑似键/值对调，禁止输出")
            return 2
        tracked = grep_count(["HEAD", "--", ":!docs/superpowers"], v)
        hits = {"树内产物": count(logs, v), "提交差异": diff.count(v), "文档": doc.count(v),
                "探针脚本": probe.count(v), "整棵tip树": grep_count(["HEAD"], v)}
        s = sum(hits.values())
        if tracked > 0:
            public_total += s
            print(f"  公开常量 >> 键 {k}（值长={len(v)}，已跟踪源码 {tracked} 处命中）：{hits}")
            continue
        total += s
        if s:
            print(f"  命中 >> 键 {k}（值长={len(v)}，已跟踪源码 0 处）：{hits}")
    print(f"判定：{len(vals)} 枚候选 × 5 个面 ⇒ 私有真值出现合计={total}（须 0）；公开常量出现合计={public_total}（另判）")
    rc = 0 if total == 0 else 1
    print(f"⇒ {'绿：无一枚非公开口令字节入档' if total == 0 else '红：立即摘字节，禁止推送'}")
    # 转写面排在判定之后：它不改 rc，但它印的是键名，必须等上面那枚形状守卫跑过才允许出声。
    if "--transcripts" in sys.argv:
        transcripts_report(vals)
    return rc


if __name__ == "__main__":
    sys.exit(main())

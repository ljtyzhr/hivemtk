#!/usr/bin/env python3
"""G7：文本编码门 —— 非 UTF-8 字节、U+FFFD 替身字符、双重编码（mojibake）签名。

为什么需要它（2026-10-12 实测抓到两处已入库的静默损坏）：
`user-server/internal/service/config_param_test.go:335` 与 `user-web/src/layout/Layout.vue:143`
的中文注释里各有连续一段 U+FFFD（字节 `EF BF BD`；`git show HEAD:` 现数：Layout 那处 2 个、
config_param_test 那处 3 个）。它们不是编译不过、不是测试不过、也不是页面报错
—— 是「注释里的一句中文永久读不通」：某个工具在写这两个文件时按「替换非法字节」的策略，
把一个汉字占的 UTF-8 字节序逐个换成了替身字符；替身落盘之后就是终态，原字符再也复原不了。
这类损坏的全部症状只有「人读到那行时才看得见」，所以没有门就等于没有发现机制。

三重来源都得防：
  1. 编辑器/脚本用 `errors=replace`、`iconv -c`、`strings.ToValidUTF8` 之类「容错写回」；
  2. 文件本身不是 UTF-8（GBK/Latin-1 混进仓）—— 严格解码直接失败；
  3. **双重编码**：UTF-8 字节被当成 Latin-1/CP1252 读出来、再存成 UTF-8，于是中文变成
     「一个带变音符的拉丁字母 + 一串控制符/标点」。它在字节层面是完全合法的 UTF-8，
     前两条判据都放行，只有签名能抓。

判据（rc=1 = 内容坏；rc=2 = 门自己坏了/前提不满足）：
  - `NOT_UTF8`   文本文件严格 UTF-8 解码失败 ⇒ 红（附首个坏字节的偏移）。**无豁免**：
                 一个不是 UTF-8 的文件里连「第几行」都不可靠，基线没法表达。
  - `FFFD`       出现 U+FFFD ⇒ 只有 `scripts/text-encoding.baseline` 里逐文件登记了
                 **出现次数**的位置放行；现算次数 > 基线 = 新增，< 基线 = STALE 红
                 （逼着把修掉的划掉，防基线烂掉）。
  - `MOJIBAKE`   出现双重编码签名 ⇒ 红。**无豁免**：建门时全仓 0 处，任何一次出现都是
                 新引入的损坏，而不是存量债。
  - rc=2：缺基线文件 / `git ls-files` 失败 / 扫描面为 0 / 基线登记的文件根本不在扫描面上
          （这四种全是「门没在扫」的形状 —— 而「没在扫」和「扫了没问题」在 rc=0 的输出里长一样）。

口径边界（别把「没数到」印成「没问题」）：
  - **文本的判定 = 前 8 KiB 无 NUL 字节**。图片/字体/压缩包因此被跳过；代价是「真文本里恰好
    含 NUL」会漏。2236 个已入库的取证 `.log` 经实测全部无 NUL、全部合法 UTF-8，
    所以它们**在面里**，没有为它们开任何豁免；
  - 扫描面 = `git ls-files --cached --others --exclude-standard`，即「已跟踪 + 未跟踪但没被忽略」。
    用这一条而不是走目录树：未跟踪又未忽略的临时件会被捞进门的面（这是 `make audit` 的 shellcheck
    门 2026-10-12 实测过的形状）。临时件里的坏编码也是真损坏，所以这正是要的；
  - 列面用 `-z`（NUL 分隔）：不带它时 git 会把非 ASCII 路径八进制转义再套引号，
    仓里 5 个中文名 md 就变成门"读不到的 5 个文件"⇒ 静默不扫（建门时实测到过，
    所以读不到的路径现在**按名字打印**，不只报计数）；
  - 列面之后读不到的文件（并行会话当场删了它）只出声、不改 rc；但基线登记的文件读不到
    或不在面上 ⇒ 退 2 —— 否则一条豁免可以靠"文件被挪走"悄悄变成"这条不再被检查"；
  - 本门自己的源码里**不举签名字符的实例**、一律写转义：否则第一个红的就是它自己
    （建门时确实红过一次，那次红的是它自己 docstring 里的示例）；
  - U+FFFD 的唯一合法用途是「我已经知道它是替身并且要把它发出去」：
    `strings.ToValidUTF8`（`user-server/internal/repository/message_hub_inbox_conversation.go:318`）
    是清洗外部不可解码文本的刻意代码，已按次数进基线。除此之外源码/文档里出现替身字符，
    一律按「某处丢了原文」处理。

`--self-check` 只喂内存里的字节夹具，断言三条判据各自能开火、干净文本不误伤、次数与行号算得对；
**不在工作树里造任何文件**。它证明的是「检测函数有牙」；「门真装上了、真在扫这棵树」仍要靠外部
反向对账（改基线、往真文件里注字符、拿 `--repo` 指空目录），过程记在计划文档的门表那一节。

执行入口：本地 `python3 scripts/check-text-encoding.py`；接 CI 时**触发 paths 必须含本脚本与基线文件**
（缺一条就是「只改判据/只动基线的提交不重跑这道门」）。
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
BASELINE_REL = Path("scripts") / "text-encoding.baseline"

#: 二进制的判据是「前 8 KiB 有没有 NUL」。
SNIFF_BYTES = 8192

# 双重编码签名的两个族（字符全部用转义写，理由见模块 docstring）：
#   - Latin-1 族：UTF-8 的后续字节（0x80-0xBF）被原样读成控制符或 ¡-¿。
#     两字节序列 ⇒ C2/C3 开头字符 + 一个 80-BF；三字节序列（汉字）⇒ E0-EF + 两个。
#   - CP1252 族：同样的后续字节被读成 euro/引号/破折号/变音符那一排，
#     是网页与编辑器里最常见的一种。
_TAIL_LATIN1 = r"[\u0080-\u00bf]"
_TAIL_CP1252 = (r"[\u00a1-\u00ff\u0081\u008d\u008f\u0090\u0092-\u009f"
                r"\u20ac\u201a\u0192\u201e\u2026\u2020\u2021\u02c6\u2030"
                r"\u0160\u2039\u0152\u017d\u2018\u2019\u201c\u201d\u2022"
                r"\u2013\u2014\u02dc\u2122\u0161\u203a\u0153\u017e\u0178]")
MOJIBAKE = re.compile(
    r"[\u00c2\u00c3]" + _TAIL_LATIN1
    + r"|[\u00e0-\u00ef]" + _TAIL_LATIN1 + "{2}"
    + r"|[\u00c2\u00c3][\u00a0-\u00ff]"
    + r"|[\u00e0-\u00ef]" + _TAIL_CP1252 + "{2}"
)


def fail(msg: str, code: int = 2) -> None:
    print(f"rc={code} {msg}", file=sys.stderr)
    raise SystemExit(code)


def scan_face(root: Path) -> list[str]:
    """`git ls-files --cached --others --exclude-standard -z` 列出的路径（相对 root）。

    必须带 `-z`：默认输出会把非 ASCII 路径**八进制转义并套上双引号**（`core.quotePath`），
    于是仓里所有中文名文件（`docs/architecture/FRP私域部署指南.md` 等 5 个）在门眼里
    是一个打不开的名字 ⇒ 被当成"读不到"跳过。建门时实测：不带 `-z` 的这版扫描面自证
    `读不到跳过 5 个`，而那 5 个是**真文件**——"没扫到"在 rc=0 的输出里和"扫了没问题"长一样。
    """
    proc = subprocess.run(
        ["git", "-C", str(root), "ls-files", "--cached", "--others",
         "--exclude-standard", "-z"],
        capture_output=True)
    if proc.returncode != 0:
        fail(f"git ls-files 失败（rc={proc.returncode}）："
             f"{proc.stderr.decode('utf-8', 'replace').strip()}")
    return [chunk.decode("utf-8", "surrogateescape")
            for chunk in proc.stdout.split(b"\0") if chunk]


def parse_baseline(path: Path) -> dict[str, int]:
    r"""`<相对路径>\t<该文件允许的 U+FFFD 次数>`；`#` 开头是注释，空行忽略。"""
    registered: dict[str, int] = {}
    for lineno, raw in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split("\t")
        if len(parts) != 2:
            parts = line.rsplit(None, 1)
        if len(parts) != 2:
            fail(f"基线第 {lineno} 行格式坏（要 `<相对路径>` 制表符 `<次数>`）：{raw}")
        rel, count = parts[0].strip(), parts[1].strip()
        if not count.isdigit():
            fail(f"基线第 {lineno} 行次数不是数字：{raw}")
        if rel in registered:
            fail(f"基线重复登记文件：{rel}")
        registered[rel] = int(count)
    return registered


def inspect_bytes(data: bytes) -> tuple[str | None, int, int]:
    """返回 (违规类别, 违规位置, U+FFFD 次数)。类别为 None 表示这个文件干净。

    位置对 `NOT_UTF8` 是首个坏字节的偏移，对 `MOJIBAKE` 是命中处在解码文本里的下标，
    对 `FFFD` 直接取成次数本身（调用方要行号再调 `fffd_lines`）。
    """
    if b"\x00" in data[:SNIFF_BYTES]:
        return "BINARY", 0, 0
    try:
        text = data.decode("utf-8")
    except UnicodeDecodeError as exc:
        return "NOT_UTF8", exc.start, 0
    n = text.count("\ufffd")
    if n:
        return "FFFD", n, n
    m = MOJIBAKE.search(text)
    if m:
        return "MOJIBAKE", m.start(), n
    return None, 0, n


def fffd_lines(data: bytes) -> list[int]:
    """U+FFFD 出现的行号（1-based），把红因指到人能直接打开的位置。"""
    return [i for i, line in enumerate(
        data.decode("utf-8").split("\n"), 1) if "\ufffd" in line]


def run_self_check() -> int:
    """判据的「有牙」证明：只用内存字节夹具，不碰工作树。"""
    gbk = "中文注释".encode("gbk")
    latin1_mojibake = "中文".encode("utf-8").decode("latin-1").encode("utf-8")
    cp1252_mojibake = "\u00e2\u20ac\u201d\u00e5\u008d\u00a0".encode("utf-8")
    clean = ("// 普通中文注释，含真重音字母 é è ü ß ñ ç ã õ Å æ œ ° ± "
             "½ ¿ · 与引号 ‘ ’ “ ” — – … ‰ € ™").encode("utf-8")
    cases = [
        ("NOT_UTF8 要开火（GBK 中文）", gbk, ("NOT_UTF8",)),
        ("NOT_UTF8 要开火（截断的多字节）", "中文".encode("utf-8")[:-1],
         ("NOT_UTF8",)),
        ("FFFD 要开火", "// 一句被换掉的中文\ufffd\ufffd 结尾".encode("utf-8"),
         ("FFFD",)),
        ("MOJIBAKE 要开火（Latin-1 族）", latin1_mojibake, ("MOJIBAKE",)),
        ("MOJIBAKE 要开火（CP1252 族）", cp1252_mojibake, ("MOJIBAKE",)),
        ("干净文本不误伤", clean, (None,)),
        ("二进制按嗅探跳过",
         b"\x89PNG\x00\x01\x02" + "中文".encode("utf-8"), ("BINARY",)),
    ]
    bad = 0
    for name, data, expect in cases:
        got = inspect_bytes(data)[0]
        if got not in expect:
            print(f"  SELF-CHECK 失败：{name} 实际判成 {got}，期望其中之一 {expect}")
            bad += 1
    # 次数与行号要一起断：只断类别的话，count() 写成「去重字符数」也是绿的。
    if inspect_bytes("a\ufffdb\ufffd".encode("utf-8"))[2] != 2:
        print("  SELF-CHECK 失败：FFFD 次数应为 2")
        bad += 1
    if fffd_lines("x\ny\ufffd\nz".encode("utf-8")) != [2]:
        print("  SELF-CHECK 失败：行号定位没指到第 2 行")
        bad += 1
    if bad:
        return 1
    print(f"OK 自检：{len(cases)} 个字节夹具 + 次数与行号定位各 1 条，判据都有牙")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", default=str(REPO_ROOT),
                    help="要扫的树（默认本脚本所在仓库根）")
    ap.add_argument("--self-check", action="store_true",
                    help="只跑内存夹具，证明判据各自能开火")
    args = ap.parse_args()

    if args.self_check:
        return run_self_check()

    root = Path(args.repo).resolve()
    baseline = root / BASELINE_REL
    if not baseline.is_file():
        fail(f"缺基线文件 {baseline}（缺基线时「零命中」与「没扫」分不开）")

    face = scan_face(root)
    if not face:
        fail(f"扫描面为 0：{root} 里 git 一个文件都没列出来，本门的读数不可信")

    registered = parse_baseline(baseline)
    face_set = set(face)
    missing = sorted(rel for rel in registered if rel not in face_set)
    if missing:
        fail("基线登记的文件不在扫描面上：" + ", ".join(missing)
             + " —— 要么文件被挪走/删了（基线该划掉），要么门这次根本没扫到它")

    not_utf8: list[tuple[str, int]] = []
    fffd_over: list[tuple[str, int, int, list[int]]] = []
    mojibake: list[tuple[str, int]] = []
    stale: list[tuple[str, int, int]] = []
    scanned = binary = hit_fffd = 0
    unreadable: list[str] = []

    for rel in face:
        try:
            data = (root / rel).read_bytes()
        except OSError:
            unreadable.append(rel)
            continue
        kind, pos, n = inspect_bytes(data)
        if kind == "BINARY":
            binary += 1
            continue
        scanned += 1
        if kind == "NOT_UTF8":
            not_utf8.append((rel, pos))
            continue
        if kind == "MOJIBAKE":
            mojibake.append((rel, pos))
            continue
        if kind == "FFFD":
            hit_fffd += 1
            allow = registered.get(rel, 0)
            if n > allow:
                fffd_over.append((rel, n, allow, fffd_lines(data)))
            elif rel in registered and n < allow:
                stale.append((rel, allow, n))

    # 基线登记的文件在面上却打不开 ⇒ 门没扫到它（这条和「不在面上」同性质）：
    # 少了它，一条豁免就能靠「名字被转义 / 文件被挪走」悄悄变成「这条不再被检查」。
    blocked = sorted(rel for rel in registered if rel in set(unreadable))
    if blocked:
        fail("基线登记的文件读不到：" + ", ".join(blocked)
             + " —— 豁免对象没被扫到，本门的读数不可信")

    print(f"扫描面: git 列出 {len(face)} 个路径 / 文本 {scanned} 个 / "
          f"二进制跳过 {binary} 个 / 读不到跳过 {len(unreadable)} 个")
    print(f"读数: NOT_UTF8={len(not_utf8)} FFFD(超出基线)={len(fffd_over)} "
          f"MOJIBAKE={len(mojibake)} STALE={len(stale)} / "
          f"含替身字符的文件 {hit_fffd} 个 / 基线登记 {len(registered)} 处")
    # 读不到的名字要打出来，不能只报计数：上一版就是只报「跳过 5 个」，
    # 而那 5 个是被 git 转义掉的中文名真文件 —— 计数看不出"没扫"，名字一眼就看出来。
    for rel in unreadable[:20]:
        print(f"  UNREADABLE: {rel} 列在面上但打不开（并行会话刚删了它？还是路径被转义了？）")

    rc = 0
    for rel, pos in not_utf8:
        rc = 1
        print(f"  NOT_UTF8: {rel} 第 {pos} 字节起不是合法 UTF-8；"
              "请用 UTF-8 重新保存（不许用 iconv -c / errors=replace 糊过去）")
    for rel, n, allow, lines in fffd_over:
        rc = 1
        print(f"  FFFD: {rel} 有 {n} 个 U+FFFD 替身字符（基线允许 {allow}），行号 {lines}"
              " —— 原文已经丢了，得把那句话重写，不能只删字符")
    for rel, pos in mojibake:
        rc = 1
        print(f"  MOJIBAKE: {rel} 第 {pos} 个字符起是双重编码签名"
              "（UTF-8 被当 Latin-1/CP1252 转写后又存成了 UTF-8）")
    for rel, allow, n in stale:
        rc = 1
        print(f"  STALE: {rel} 基线登记 {allow} 处，现算 {n} ⇒ 把修掉的从基线里划掉")

    if rc == 0:
        print("OK 文本编码门：全树文本文件均为合法 UTF-8，替身字符次数与基线一致，"
              "无双重编码签名")
    return rc


if __name__ == "__main__":
    sys.exit(main())

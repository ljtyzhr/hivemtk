#!/usr/bin/env python3
"""A2-b：SBOM 产物断言 —— 证明「产物确实被生成、确实被上传、确实覆盖了本仓的依赖」。

为什么需要它（以及它治的是哪种假绿）：
  `sbom.yml` 从接入起一直在跑，也一直在绿——但它的绿只说明「三个 syft 命令的退出码是 0」。
  这中间有太多可以「成功地产出一个没用的东西」的形态：
    · syft 换成 `table` 格式 ⇒ 产物是纯文本，下游把它当 SPDX 解析时才炸（而那时已经没有 CI 了）；
    · 三份 scan 里有一份 `working-directory` 写错，实际扫了整个仓 ⇒ 三份产物内容雷同，
      「bridge 的 SBOM」名不副实，而退出码依然是 0；
    · 生成路径和上传路径不一致（改名只改一处）⇒ 产物躺在 runner 上，artifact 里没有它，
      下一次有人真去取 SBOM 才发现——而 SBOM 的用途恰恰是「出了事的时候」；
    · syft 一条依赖都没解析出来（目录空、图案没匹配上）⇒ SPDX 结构合法、零 package，
      校验「文件存在且是 JSON」的检查全绿。
  所以本门分两段，缺一不可：
    `--workflow`：**静态**核对工作流本身——谁生成、生成到哪、谁上传、上传的是不是同一批；
    `--dir`：**产物级**核对该产物确实覆盖了本仓清单里声明的直接依赖（不覆盖 ⇒ 它不是本仓的 SBOM）。

判据（任一不成立即 rc=1）：
  工作流段：
   A. 期望的产物清单（`EXPECTED`）逐份必须有一个 syft 步骤真的生成它 ⇒ `MISSING_ARTIFACT`；
   B. 每个 syft 步骤必须显式声明 `working-directory` ⇒ `NO_WORKDIR`
      （不写＝在仓根扫，三份「分域」SBOM 实际是同一坨全仓快照）；
   C. 每步只许一条 `spdx-json` 输出 ⇒ `DUPLICATE_SCAN`（OPT-CI-02 那一批一模一样的两行）；
   D. 输出格式必须是 `spdx-json` ⇒ `NOT_SPDX`（`syft-json`/`table` 不是 SPDX，下游解析不了）；
   E. 输出路径不得落在被扫描目录内 ⇒ `OUTPUT_INSIDE_SCOPE`
      （否则第二份扫描会把第一份产物当输入编进去，产物自指、越滚越大）；
   F. 每个生成路径都被某个 upload 步骤的 `path` 收录 ⇒ `ORPHAN_ARTIFACT`；
   G. 每个 upload 路径都有对应的生成步骤 ⇒ `GHOST_UPLOAD`（上传一个不存在的路径是**静默**的：
      upload-artifact 默认 warn 不 fail）;
   H. upload 步骤必须设 `retention-days` ⇒ `NO_RETENTION`（不写＝默认 90 天，但这是仓库策略，
      不该由默认值决定；且它显式存在时才看得出「有人想过保留多久」）；
   I. 同一 (working-directory, 输出) 出现两次 ⇒ `DUPLICATE_ARTIFACT`；
   J. 提交发行附件的步骤必须要求 `contents: write` ⇒ `RELEASE_NEEDS_WRITE`
      （本仓踩过：`sbom.yml` 权限只有 `contents: read` 却挂着 `action-gh-release`，
      在 tag 上必然 403；因为一个 tag 都没推过，它零执行史、从未暴露）。
  产物段：
   K. `EXPECTED` 里每份产物必须存在于 `--dir` ⇒ `MISSING_FILE`；
   L. 必须是可解析 JSON、有 `spdxVersion`、`packages` 非空 ⇒ `BAD_SPDX` / `EMPTY_SBOM`；
   M. `SPDXID` 不得重复、`name` 不得为空 ⇒ `BAD_PACKAGE`；
   N. 该产物对应清单里声明的**直接依赖**必须全部出现在产物里 ⇒ `MISSING_DEP`
      （这条才真正回答「这份 SBOM 是不是本仓的」；A/L 只回答「有个文件」）；
   O. 产物大小 < 1KB ⇒ `SUSPICIOUS_SIZE`（结构合法但近似空壳的常见形态）。

口径边界（把"没数到"写成"没数到"）：
  - **只对账直接依赖**：`go.mod` 不带 `// indirect` 的行、`package.json` 的 `dependencies`。
    devDependencies 与传递依赖只打印数额、不判定——syft 的各个 cataloger 对这两类的收录口径
    不一致（`node_modules` 在场与不在场时结果不同），把它们写成硬判据会得到一道
    「本机能过、CI 上红」或反之的门。
  - 名称匹配走 purl（`externalRefs[].referenceLocator`，如 `pkg:golang/github.com/gin-gonic/gin@v1.12.0`）
    优先、`name` 兜底，两侧都归一化（去 `pkg:` 前缀、去版本段、URL 解码、小写）。
    只做**精确匹配**，不做模糊/后缀匹配：模糊匹配会把「少了 `github.com/x/y`」变成
    「因为有 `example.com/y` 所以放行」，那正是这类门最常见的退化。
  - `--workflow` 只看**执行位**：`run:` 块里的 syft 命令。注释里提到的命令不算
    （本仓 `sbom.yml` 的历史说明就是注释，剥不干净会把已删掉的步骤当成还在）。
  - 本门**不生成** SBOM、不联网、不需要 syft 在 PATH 里。产物段是给「已经生成完之后」
    的断言用；`sbom.yml` 在「Generate」之后、`upload` 之前调用它（那里 syft 已装、产物已在）。

执行入口：
  本地 `python3 scripts/check-sbom-artifacts.py --workflow`（只需 PyYAML，不需要 syft）
        `python3 scripts/check-sbom-artifacts.py --dir .`（有产物时）
  CI：`sbom.yml` 的 gate 步骤 + `user-server-ci.yml` 的 static gates。
牙齿：`python3 scripts/check-sbom-artifacts.py --self-check`（13 枚控制，A–O 每类至少一枚正向 + 极性对）。
"""

from __future__ import annotations

import argparse
import json
import re
import sys
from pathlib import Path
from urllib.parse import unquote

try:
    import yaml
except ImportError:  # 环境前提缺位 ≠ 判据成立
    print("ENV-BROKEN：缺 PyYAML（无法解析工作流，本趟未判定）", file=sys.stderr)
    sys.exit(2)

REPO_ROOT = Path(__file__).resolve().parent.parent
for _i, _a in enumerate(sys.argv):
    if _a == "--repo" and _i + 1 < len(sys.argv):
        REPO_ROOT = Path(sys.argv[_i + 1]).resolve()

WORKFLOW_REL = Path(".github") / "workflows" / "sbom.yml"

# 期望的三份产物：文件名 → (被扫描目录, 该域的直接依赖清单)。
# 刻意列死而不是「扫一遍工作流里出现的 .spdx.json」：后者只能证明「生成的和上传的一致」，
# 不能证明「该有的都在」——删掉整段 bridge 扫描它也会绿。
EXPECTED: list[tuple[str, str, str]] = [
    ("sbom-go.spdx.json", "user-server", "user-server/go.mod"),
    ("sbom-bridge.spdx.json", "user-web/bridge", "user-web/bridge/package.json"),
    ("sbom-frontend.spdx.json", "user-web", "user-web/package.json"),
]

SCAN_CMD = re.compile(r"^\s*syft\s+scan\b")
OUT_ARG = re.compile(r"(?:-o|--output|--outputs|-o=)[\s=]+([\w.-]+=[^\s'\"]+)")
RELEASE_USES = re.compile(r"(action-gh-release|actions/create-release|softprops/action-gh-release)", re.I)
MIN_ARTIFACT_BYTES = 1024


class Finding:
    def __init__(self, code: str, msg: str):
        self.code = code
        self.msg = msg

    def __str__(self) -> str:
        return f"  {self.code}: {self.msg}"


# ── 工作流段 ────────────────────────────────────────────────────────────────
def strip_comment_lines(block: str) -> list[str]:
    """run 块里只留会真正执行的行。注释掉的命令不是执行站点。"""
    return [ln for ln in block.splitlines() if ln.strip() and not ln.lstrip().startswith("#")]


def scan_outputs(steps: list[dict]) -> list[dict]:
    """扫出所有 syft 扫描站点：{step, workdir, outs:[(fmt, path)], raw}。

    `workdir` 是原始字符串（可能为空——那正是要报的洞），不做补全。
    """
    sites: list[dict] = []
    for idx, step in enumerate(steps):
        run = step.get("run")
        if not isinstance(run, str):
            continue
        lines = strip_comment_lines(run)
        scans = [ln for ln in lines if SCAN_CMD.match(ln)]
        if not scans:
            continue
        outs: list[tuple[str, str]] = []
        for ln in scans:
            for spec in OUT_ARG.findall(ln):
                fmt, _, path = spec.partition("=")
                outs.append((fmt.strip(), path.strip()))
        sites.append({
            "index": idx,
            "name": str(step.get("name") or f"step#{idx}"),
            "workdir": step.get("working-directory"),
            "outs": outs,
            "scan_count": len(scans),
        })
    return sites


def resolve(base: str, path: str) -> str:
    """把相对 working-directory 的产物路径归一成仓相对路径（不带前导 ./）。"""
    from posixpath import normpath
    return normpath(f"{base.rstrip('/')}/{path}" if base else path).lstrip("/")


def upload_paths(steps: list[dict]) -> list[dict]:
    """扫出所有 upload-artifact 站点：{step, name, paths, retention}。"""
    out: list[dict] = []
    for idx, step in enumerate(steps):
        uses = str(step.get("uses") or "")
        if "upload-artifact" not in uses:
            continue
        with_ = step.get("with") or {}
        raw = with_.get("path")
        if raw is None:
            paths: list[str] = []
        elif isinstance(raw, str):
            paths = [x.strip() for x in raw.splitlines() if x.strip()]
        else:
            paths = [str(x).strip() for x in raw if str(x).strip()]
        out.append({
            "index": idx,
            "name": str(step.get("name") or f"step#{idx}"),
            "artifact": str(with_.get("name") or ""),
            "paths": paths,
            "retention": with_.get("retention-days"),
        })
    return out


def check_workflow(doc: dict) -> tuple[list[Finding], dict[str, int]]:
    """工作流静态判据 A–J。doc 是 yaml.safe_load 的结果。"""
    findings: list[Finding] = []
    stats = {"scan_sites": 0, "artifacts": 0, "uploads": 0}

    jobs = doc.get("jobs") or {}
    all_steps: list[dict] = []
    for job in jobs.values():
        if isinstance(job, dict):
            all_steps.extend(s for s in (job.get("steps") or []) if isinstance(s, dict))

    sites = scan_outputs(all_steps)
    ups = upload_paths(all_steps)
    stats["scan_sites"] = len(sites)
    stats["uploads"] = len(ups)

    generated: dict[str, str] = {}   # 仓相对产物路径 → 生成它的步骤名
    seen_pairs: dict[tuple[str, str], str] = {}

    for site in sites:
        label = site["name"]
        wd = site["workdir"]
        if not wd:
            findings.append(Finding(
                "NO_WORKDIR", f"{label}: syft 扫描步骤没有 working-directory —— "
                              f"实际扫的是仓根，这一份产物就不再是它宣称的那个域"))
        base = str(wd or "")
        if site["scan_count"] > 1:
            findings.append(Finding(
                "DUPLICATE_SCAN", f"{label}: 同一步里跑了 {site['scan_count']} 条 syft 命令 —— "
                                  f"同一份输入扫两遍只会得到两个同名产物，后者覆盖前者"))
        if not site["outs"]:
            findings.append(Finding(
                "NOT_SPDX", f"{label}: syft 步骤没有 `-o spdx-json=<path>` 输出参数 —— "
                            f"没有产物就无从断言"))
        for fmt, path in site["outs"]:
            if fmt != "spdx-json":
                findings.append(Finding(
                    "NOT_SPDX", f"{label}: 输出格式是 `{fmt}` 而不是 `spdx-json` —— "
                                f"下游按 SPDX 解析，格式不对时 CI 是绿的、用的时候才炸"))
            rel_out = resolve(base, path)
            if base and (rel_out == base.rstrip("/") or rel_out.startswith(base.rstrip("/") + "/")):
                findings.append(Finding(
                    "OUTPUT_INSIDE_SCOPE", f"{label}: 产物 {rel_out} 落在被扫描目录 {base} 内 —— "
                                           f"下一份扫描会把它当输入编进去（产物自指）"))
            pair = (base, rel_out)
            if pair in seen_pairs:
                findings.append(Finding(
                    "DUPLICATE_ARTIFACT", f"{label}: 与 {seen_pairs[pair]} 生成同一产物 {rel_out}"))
            seen_pairs[pair] = label
            generated[rel_out] = label

    stats["artifacts"] = len(generated)
    for fname, _scope, _manifest in EXPECTED:
        if fname not in generated:
            findings.append(Finding(
                "MISSING_ARTIFACT", f"{fname}: 工作流里没有任何 syft 步骤生成它 —— "
                                    f"期望的三份产物（go / bridge / frontend）缺一份就不算交付"))

    uploaded: dict[str, str] = {}
    for up in ups:
        if up["retention"] in (None, ""):
            findings.append(Finding(
                "NO_RETENTION", f"{up['name']}: upload-artifact 没写 retention-days —— "
                                f"保留多久是仓库策略，不该由 action 的默认值决定"))
        for p in up["paths"]:
            uploaded[resolve("", p)] = up["name"]

    for rel_out, label in sorted(generated.items()):
        if rel_out not in uploaded:
            findings.append(Finding(
                "ORPHAN_ARTIFACT", f"{rel_out}（{label} 生成）没有被任何 upload 步骤收录 —— "
                                   f"产物躺在 runner 上，artifact 里没有它"))
    for rel_out, label in sorted(uploaded.items()):
        if rel_out not in generated:
            findings.append(Finding(
                "GHOST_UPLOAD", f"{label} 上传了 {rel_out}，但没有任何 syft 步骤生成它 —— "
                                f"upload-artifact 对不存在的路径只是 warning，这一步会绿"))

    # 权限与发行附件：read-only 工作流里挂 gh-release 必然 403，且只在 tag 上才会暴露。
    perms = doc.get("permissions")
    write_ok = False
    if isinstance(perms, dict):
        write_ok = str(perms.get("contents", "")).lower() == "write"
    elif str(perms) == "write-all":
        write_ok = True
    if not write_ok:
        for job_id, job in jobs.items():
            if not isinstance(job, dict):
                continue
            jp = job.get("permissions")
            if isinstance(jp, dict) and str(jp.get("contents", "")).lower() == "write":
                write_ok = True
                break
    if not write_ok:
        for step in all_steps:
            uses = str(step.get("uses") or "")
            if RELEASE_USES.search(uses):
                findings.append(Finding(
                    "RELEASE_NEEDS_WRITE", f"步骤 `{step.get('name') or uses}` 会创建/更新 Release，"
                                           f"但工作流权限只有只读 contents —— 在 tag 上必然 403，"
                                           f"而 push 上永远看不到（零执行史）"))
    return findings, stats


def load_workflow(path: Path) -> dict:
    if not path.is_file():
        fail(f"找不到工作流：{path}")
    doc = yaml.safe_load(path.read_text(encoding="utf-8"))
    if not isinstance(doc, dict):
        fail(f"工作流顶层不是映射：{path}")
    return doc


# ── 产物段 ──────────────────────────────────────────────────────────────────
def normalize_name(raw: str) -> str:
    """purl / 包名 → 可比的归一化串。"""
    s = unquote(str(raw or "")).strip().lower()
    if s.startswith("pkg:"):
        s = s[len("pkg:"):]
        # `golang/github.com/x/y@v1` → `github.com/x/y`
        kind, _, rest = s.partition("/")
        s = rest if kind in ("golang", "npm", "gem", "pypi", "cargo") else s
    if "@" in s:
        s = s.split("@", 1)[0]
    return s.rstrip("/")


def sbom_names(doc: dict) -> set[str]:
    """产物里所有包的可比名集合（purl 优先，name 兜底）。"""
    names: set[str] = set()
    for pkg in doc.get("packages") or []:
        if not isinstance(pkg, dict):
            continue
        for ref in pkg.get("externalRefs") or []:
            if isinstance(ref, dict):
                loc = ref.get("referenceLocator")
                if isinstance(loc, str) and loc.startswith("pkg:"):
                    names.add(normalize_name(loc))
        if pkg.get("name"):
            names.add(normalize_name(pkg["name"]))
    return names


def manifest_deps(path: Path) -> list[str] | None:
    """清单里的**直接依赖**名。go.mod 不带 `// indirect`；package.json 只取 dependencies。

    返回 None 表示**清单读不到**（文件没了/JSON 坏了）——那与「读到但为空」是两种
    判据：前者红（`MISSING_FILE`），后者是合法形状（bridge 的纯 MV3 扩展运行时
    零依赖），由调用方打印说明后跳过对账。
    """
    if not path.is_file():
        return None
    text = path.read_text(encoding="utf-8", errors="replace")
    if path.name == "go.mod":
        out: list[str] = []
        in_block = False
        for raw in text.splitlines():
            line = raw.split("//")[0].strip()
            if not line:
                continue
            if line.startswith("require ("):
                in_block = True
                continue
            if in_block and line == ")":
                in_block = False
                continue
            if in_block:
                body = line
            elif line.startswith("require "):
                body = line[len("require "):].strip()
            else:
                continue
            if "// indirect" in raw:
                continue
            parts = body.split()
            if len(parts) >= 2:
                out.append(parts[0])
        return out
    try:
        data = json.loads(text)
    except json.JSONDecodeError:
        return None
    return sorted((data.get("dependencies") or {}).keys())


def check_artifacts(root: Path) -> tuple[list[Finding], dict[str, int]]:
    """产物级判据 K–O。root 是产物所在目录（CI 里是 workspace 根）。"""
    findings: list[Finding] = []
    stats = {"files": 0, "packages": 0, "deps_checked": 0}

    for fname, _scope, manifest_rel in EXPECTED:
        p = root / fname
        if not p.is_file():
            findings.append(Finding(
                "MISSING_FILE", f"{fname}: 产物不存在（{p}）—— 生成步骤没跑，或写到了别处"))
            continue
        stats["files"] += 1
        size = p.stat().st_size
        if size < MIN_ARTIFACT_BYTES:
            findings.append(Finding(
                "SUSPICIOUS_SIZE", f"{fname}: 只有 {size} 字节 —— 结构可能合法但内容近似空壳"))
        try:
            doc = json.loads(p.read_text(encoding="utf-8", errors="replace"))
        except json.JSONDecodeError as exc:
            findings.append(Finding("BAD_SPDX", f"{fname}: 不是可解析 JSON（{exc}）"))
            continue
        if not isinstance(doc, dict) or not str(doc.get("spdxVersion", "")).startswith("SPDX-"):
            findings.append(Finding(
                "BAD_SPDX", f"{fname}: 缺 spdxVersion 或不是 SPDX（拿到的是 "
                            f"{str(doc.get('spdxVersion'))[:20]!r}）"))
            continue
        pkgs = [x for x in (doc.get("packages") or []) if isinstance(x, dict)]
        stats["packages"] += len(pkgs)
        if not pkgs:
            findings.append(Finding(
                "EMPTY_SBOM", f"{fname}: SPDX 结构合法但 packages 为空 —— "
                              f"「文件存在且是 JSON」的检查在这里会全绿，而它什么都没覆盖"))
            continue
        ids = [str(x.get("SPDXID") or "") for x in pkgs]
        if len(set(ids)) != len(ids):
            findings.append(Finding("BAD_PACKAGE", f"{fname}: SPDXID 有重复（{len(ids) - len(set(ids))} 个）"))
        empty = [i for i, x in enumerate(pkgs) if not str(x.get("name") or "").strip()]
        if empty:
            findings.append(Finding("BAD_PACKAGE", f"{fname}: {len(empty)} 个 package 没有 name"))

        deps = manifest_deps(root / manifest_rel)
        if deps is None:
            findings.append(Finding(
                "MISSING_FILE", f"{fname}: 对账用的清单 {manifest_rel} 读不到 —— "
                                f"清单没了就不该继续认为这份 SBOM 覆盖了本仓"))
            continue
        if not deps:
            # 清单在读得到的前提下 dependencies 为空：这是合法形状（如 bridge 的
            # 纯 MV3 扩展——运行时零依赖，devDeps 只服务构建/测试，刻意不进判据）。
            # 什么都不判，但要打印出来：空清单的「覆盖」是空洞真，读数不能把它
            # 假装成对账过了。
            print(f"  note: {fname}: 清单 {manifest_rel} 无直接依赖（dependencies 为空），"
                  f"跳过依赖对账；产物含 {len(pkgs)} 个包（devDeps/传递依赖只打印不判）")
            continue
        stats["deps_checked"] += len(deps)
        have = sbom_names(doc)
        missing = [d for d in deps if normalize_name(d) not in have]
        if missing:
            findings.append(Finding(
                "MISSING_DEP", f"{fname}: 清单 {manifest_rel} 里 {len(missing)}/{len(deps)} 个直接依赖"
                               f"不在产物中 —— 这份 SBOM 没有覆盖本仓的依赖面；缺："
                               f"{', '.join(missing[:8])}"))
        if len(pkgs) < len(deps):
            findings.append(Finding(
                "EMPTY_SBOM", f"{fname}: 产物只有 {len(pkgs)} 个包，少于清单里的 {len(deps)} 个直接依赖 "
                              f"—— syft 这一轮基本没解析到东西"))
    return findings, stats


def fail(msg: str, code: int = 2):
    print(f"❌ {msg}")
    sys.exit(code)


# ── 自检：判据坏掉时本门会恒绿，所以每类都要自己咬一遍 ──────────────────────
GOOD_WORKFLOW = """
name: SBOM
on:
  push:
  workflow_dispatch:
permissions:
  contents: read
jobs:
  sbom:
    runs-on: ubuntu-latest
    steps:
      - name: Install syft
        run: curl -sSfL https://example.com/install.sh | sh -s -- -b /usr/local/bin
      - name: Generate SBOM (SPDX, Go)
        working-directory: user-server
        run: |
          syft scan dir:. -o spdx-json=../sbom-go.spdx.json
      - name: Generate SBOM (SPDX, Bridge)
        working-directory: user-web/bridge
        run: |
          syft scan dir:. -o spdx-json=../../sbom-bridge.spdx.json
      - name: Generate SBOM (SPDX, Frontend)
        working-directory: user-web
        run: |
          syft scan dir:. -o spdx-json=../sbom-frontend.spdx.json
      - name: Upload SBOM artifacts
        uses: actions/upload-artifact@v7
        with:
          name: sbom
          path: |
            sbom-go.spdx.json
            sbom-bridge.spdx.json
            sbom-frontend.spdx.json
          retention-days: 90
"""


def self_check() -> int:
    controls: list[tuple[str, bool]] = []

    def wf(mutate=None) -> tuple[set[str], dict[str, int]]:
        doc = yaml.safe_load(GOOD_WORKFLOW)
        if mutate:
            mutate(doc)
        findings, stats = check_workflow(doc)
        return {f.code for f in findings}, stats

    def steps(doc) -> list[dict]:
        return doc["jobs"]["sbom"]["steps"]

    # 1 极性对：干净工作流一条都不红，且三份产物都被数到
    codes, stats = wf()
    controls.append(("干净工作流 ⇒ 零 findings，且数到 3 份产物",
                     not codes and stats["artifacts"] == 3))
    # 2 working-directory 缺失
    codes, _ = wf(lambda d: steps(d)[1].pop("working-directory"))
    controls.append(("syft 步骤缺 working-directory ⇒ NO_WORKDIR", "NO_WORKDIR" in codes))
    # 3 同步两条相同扫描
    codes, _ = wf(lambda d: steps(d)[1].__setitem__(
        "run", "syft scan dir:. -o spdx-json=../a.json\nsyft scan dir:. -o spdx-json=../b.json\n"))
    controls.append(("一步两条 syft ⇒ DUPLICATE_SCAN", "DUPLICATE_SCAN" in codes))
    # 4 产物没被上传
    codes, _ = wf(lambda d: steps(d)[4]["with"].__setitem__("path", "sbom-go.spdx.json\nsbom-bridge.spdx.json\n"))
    controls.append(("产出了但没上传 ⇒ ORPHAN_ARTIFACT", "ORPHAN_ARTIFACT" in codes))
    # 5 上传了没生成
    codes, _ = wf(lambda d: steps(d)[4]["with"].__setitem__(
        "path", "sbom-go.spdx.json\nsbom-bridge.spdx.json\nsbom-frontend.spdx.json\nsbom-extra.json\n"))
    controls.append(("上传了不存在的产物 ⇒ GHOST_UPLOAD", "GHOST_UPLOAD" in codes))
    # 6 产物落在被扫描目录内
    codes, _ = wf(lambda d: steps(d)[1].__setitem__(
        "run", "syft scan dir:. -o spdx-json=./sbom-go.spdx.json\n"))
    controls.append(("输出落在扫描目录内 ⇒ OUTPUT_INSIDE_SCOPE", "OUTPUT_INSIDE_SCOPE" in codes))
    # 7 缺 retention
    codes, _ = wf(lambda d: steps(d)[4]["with"].pop("retention-days"))
    controls.append(("upload 缺 retention-days ⇒ NO_RETENTION", "NO_RETENTION" in codes))
    # 8 格式不是 SPDX
    codes, _ = wf(lambda d: steps(d)[1].__setitem__("run", "syft scan dir:. -o table=../sbom-go.spdx.json\n"))
    controls.append(("输出 table 格式 ⇒ NOT_SPDX", "NOT_SPDX" in codes))
    # 9 少一整份产物（删掉 bridge 扫描与上传行）——「该有的都在」不是「生成=上传」
    def _drop_bridge(d):
        st = steps(d)
        del st[2]
        st[3]["with"]["path"] = "sbom-go.spdx.json\nsbom-frontend.spdx.json\n"
    codes, _ = wf(_drop_bridge)
    controls.append(("少一份期望产物 ⇒ MISSING_ARTIFACT", "MISSING_ARTIFACT" in codes))
    # 10 只读权限下挂 gh-release
    codes, _ = wf(lambda d: steps(d).append(
        {"name": "Attach", "uses": "softprops/action-gh-release@v2", "with": {"files": "sbom-go.spdx.json"}}))
    controls.append(("只读 contents 挂 gh-release ⇒ RELEASE_NEEDS_WRITE", "RELEASE_NEEDS_WRITE" in codes))
    # 11 上面那条的反向：给了 contents: write 就不该红
    def _write_perm(d):
        d["permissions"] = {"contents": "write"}
        steps(d).append({"name": "Attach", "uses": "softprops/action-gh-release@v2"})
    codes, _ = wf(_write_perm)
    controls.append(("contents: write 时挂 gh-release ⇒ 不红（极性对）", "RELEASE_NEEDS_WRITE" not in codes))

    # ── 产物段 ──
    import tempfile

    def artifact_checks(go_names: list[str], pkgs_extra: int = 5, spdx_version: str = "SPDX-2.3",
                        empty: bool = False):
        root = Path(tempfile.mkdtemp())
        (root / "user-server").mkdir(parents=True, exist_ok=True)
        (root / "user-web" / "bridge").mkdir(parents=True, exist_ok=True)
        (root / "user-server" / "go.mod").write_text(
            "module hivemtk-user\n\ngo 1.26.0\n\nrequire (\n" +
            "".join(f"\t{n} v1.0.0\n" for n in go_names) +
            "\tgithub.com/indirect/only v1.0.0 // indirect\n)\n", encoding="utf-8")
        (root / "user-web" / "package.json").write_text(
            json.dumps({"dependencies": {"vue": "^3.5.0"}}), encoding="utf-8")
        (root / "user-web" / "bridge" / "package.json").write_text(
            json.dumps({"dependencies": {"webextension-polyfill": "^0.12.0"}}), encoding="utf-8")
        for fname, scope, _m in EXPECTED:
            if scope == "user-server":
                names = go_names
            elif scope == "user-web/bridge":
                names = ["webextension-polyfill"]
            else:
                names = ["vue"]
            packages = [] if empty else [
                {"name": "hivemtk-dir", "SPDXID": "SPDXRef-DOCUMENT", "versionInfo": "0"}] + [
                {"name": n, "SPDXID": f"SPDXRef-{i}", "versionInfo": "1.0.0",
                 "externalRefs": [{"referenceLocator":
                                   f"pkg:{'golang' if '/' in n else 'npm'}/{n}@v1.0.0"}]}
                for i, n in enumerate(names)] + [
                {"name": f"filler-{i}", "SPDXID": f"SPDXRef-f{i}"} for i in range(pkgs_extra)]
            doc = {"spdxVersion": spdx_version, "SPDXID": "SPDXRef-DOCUMENT",
                   "creationInfo": {"created": "2026-10-24T00:00:00Z"}, "packages": packages}
            body = json.dumps(doc)
            if len(body) < MIN_ARTIFACT_BYTES:
                body = body[:-1] + ", \"_pad\": \"" + "x" * (MIN_ARTIFACT_BYTES - len(body) + 50) + "\"}"
            (root / fname).write_text(body, encoding="utf-8")
        findings, stats = check_artifacts(root)
        return {f.code for f in findings}, stats

    # 12 产物覆盖全部直接依赖 ⇒ 零 findings（极性对）
    codes, stats = artifact_checks(["github.com/gin-gonic/gin", "gorm.io/gorm"])
    controls.append(("产物覆盖全部直接依赖 ⇒ 零 findings（间接依赖不参与）",
                     not codes and stats["deps_checked"] == 4))
    # 13 少一个直接依赖 ⇒ MISSING_DEP（这条是本门存在的理由）
    root = Path(tempfile.mkdtemp())
    (root / "user-server").mkdir(parents=True, exist_ok=True)
    (root / "user-web" / "bridge").mkdir(parents=True, exist_ok=True)
    (root / "user-server" / "go.mod").write_text(
        "module m\n\nrequire (\n\tgithub.com/gin-gonic/gin v1.12.0\n\tgithub.com/absent/dep v1.0.0\n)\n",
        encoding="utf-8")
    (root / "user-web" / "package.json").write_text(json.dumps({"dependencies": {"vue": "^3.5.0"}}), encoding="utf-8")
    (root / "user-web" / "bridge" / "package.json").write_text(
        json.dumps({"dependencies": {"webextension-polyfill": "^0.12.0"}}), encoding="utf-8")
    for fname, scope, _m in EXPECTED:
        names = (["github.com/gin-gonic/gin"] if scope == "user-server"
                 else ["webextension-polyfill"] if scope == "user-web/bridge" else ["vue"])
        pkgs = [{"name": n, "SPDXID": f"SPDXRef-{i}"} for i, n in enumerate(names)]
        pkgs += [{"name": f"pad-{i}", "SPDXID": f"SPDXRef-p{i}"} for i in range(5)]
        body = json.dumps({"spdxVersion": "SPDX-2.3", "packages": pkgs})
        if len(body) < MIN_ARTIFACT_BYTES:
            body = body[:-1] + ", \"_pad\": \"" + "x" * MIN_ARTIFACT_BYTES + "\"}"
        (root / fname).write_text(body, encoding="utf-8")
    findings, _ = check_artifacts(root)
    controls.append(("缺一个直接依赖 ⇒ MISSING_DEP", "MISSING_DEP" in {f.code for f in findings}))
    # 14 packages 为空 ⇒ EMPTY_SBOM（结构合法但什么都没覆盖）
    codes, _ = artifact_checks([], empty=True)
    controls.append(("packages 为空 ⇒ EMPTY_SBOM", "EMPTY_SBOM" in codes))
    # 15 不是 SPDX ⇒ BAD_SPDX
    codes, _ = artifact_checks(["github.com/gin-gonic/gin"], spdx_version="")
    controls.append(("缺 spdxVersion ⇒ BAD_SPDX", "BAD_SPDX" in codes))
    # 16 产物缺失 ⇒ MISSING_FILE
    root = Path(tempfile.mkdtemp())
    findings, _ = check_artifacts(root)
    controls.append(("三份产物都不在 ⇒ MISSING_FILE", "MISSING_FILE" in {f.code for f in findings}))

    failed = [n for n, ok in controls if not ok]
    for name, ok in controls:
        print(f"  {'✓' if ok else '✗'} {name}")
    print(f"自检：{len(controls)} 枚控制，失败 {len(failed)} 枚")
    return 0 if not failed else 1


def main() -> int:
    if "--self-check" in sys.argv:
        return self_check()

    ap = argparse.ArgumentParser(description="SBOM 产物断言（A2-b）")
    ap.add_argument("--workflow", nargs="?", const=str(WORKFLOW_REL), default=None,
                    help="只做工作流静态判据（默认读 .github/workflows/sbom.yml）")
    ap.add_argument("--dir", default=None, help="只做产物级判据，参数是产物所在目录")
    ap.add_argument("--repo", default=".", help="仓库根（推导期望清单用）")
    args = ap.parse_args()

    if args.workflow is None and args.dir is None:
        # 什么都不给＝两段都跑（本地默认口径）。产物不存在时产物段会报 MISSING_FILE，
        # 所以 CI 里要分段调用：静态 gates 只跑 --workflow。
        args.workflow = str(WORKFLOW_REL)
        if (REPO_ROOT / EXPECTED[0][0]).is_file():
            args.dir = "."

    rc = 0
    if args.workflow is not None:
        doc = load_workflow(REPO_ROOT / args.workflow)
        findings, stats = check_workflow(doc)
        print(f"工作流 {args.workflow}: syft 扫描站点 {stats['scan_sites']} 处 / "
              f"产物 {stats['artifacts']} 份 / upload 步骤 {stats['uploads']} 个")
        if findings:
            rc = 1
            for f in findings:
                print(f)
            print("❌ 工作流段未通过")
        else:
            print("✅ 工作流段通过：三份产物都被生成、都被上传，格式/作用域/权限一致")
    if args.dir is not None:
        findings, stats = check_artifacts(Path(args.dir).resolve())
        print(f"产物目录 {args.dir}: 文件 {stats['files']} 份 / 包 {stats['packages']} 个 / "
              f"对账直接依赖 {stats['deps_checked']} 条")
        if findings:
            rc = 1
            for f in findings:
                print(f)
            print("❌ 产物段未通过")
        else:
            print("✅ 产物段通过：每份 SPDX 都覆盖了对应清单的全部直接依赖")
    return rc


if __name__ == "__main__":
    sys.exit(main())

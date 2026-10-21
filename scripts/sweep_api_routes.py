#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""sweep_api_routes.py — 任务三逐路由 curl 验证的**辅助分类器**。

它不替代「逐条手工 curl」：请求仍然由人（或人驱动的循环）逐条发出，
本脚本只做一件事——把响应体归类到「通过 / 设计内拒绝 / 真缺陷」三档，
让人把注意力放在真缺陷上。

用法：
  python3 scripts/sweep_api_routes.py <响应JSON文件>
  每个文件一行：一个 JSON 对象 {"url":..., "method":..., "body":...}
"""
import json
import re
import sys

# 设计内的业务拒绝（不是缺陷）：功能开关关闭、订阅闸、底座未装配、鉴权、
# 参数校验、幂等冲突、资源不存在。
EXPECTED_REJECT = {
    "SERVICE_UNAVAILABLE_6003",   # 底座未装配（功能开关 off）
    "403", "404", "400", "409", "401",
    "4301",                        # 功能模块未开通
    "4305",                        # 订阅到期
    "PARAM_ERROR", "NOT_FOUND", "FORBIDDEN", "UNAUTHORIZED", "CONFLICT",
}


def classify(body_text, http_status):
    try:
        d = json.loads(body_text)
    except Exception:
        # 响应体被截断（超过落盘上限）⇒ 仍按前缀判 code，避免把好路由误判成"非 JSON"
        m = re.search(r'"code"\s*:\s*("?)([0-9A-Za-z_]+)\1', body_text[:120])
        if m:
            code = m.group(2)
            if code in ("0", "SUCCESS"):
                return "通过", "(响应体截断)"
            if code in EXPECTED_REJECT:
                return "设计内拒绝", code
            return "待查", f"code={code}（响应体截断）"
        return "非 JSON", body_text[:160]
    if not isinstance(d, dict):
        return "非 JSON", body_text[:160]
    code = d.get("code")
    if code == 0 or code == "0" or code == "SUCCESS":
        return "通过", ""
    # 信封里的 code 是数字或字符串都算业务码
    if str(code) in EXPECTED_REJECT:
        return "设计内拒绝", str(d.get("message", ""))[:120]
    if http_status in (401, 403, 404):
        return "设计内拒绝", f"HTTP {http_status} {str(d.get('message',''))[:100]}"
    return "待查", f"code={code} msg={str(d.get('message',''))[:140]}"


if __name__ == "__main__":
    for line in open(sys.argv[1], encoding="utf-8"):
        line = line.strip()
        if not line:
            continue
        rec = json.loads(line)
        verdict, detail = classify(rec.get("body", ""), rec.get("status", 200))
        rec["verdict"] = verdict
        rec["detail"] = detail
        print(json.dumps(rec, ensure_ascii=False))

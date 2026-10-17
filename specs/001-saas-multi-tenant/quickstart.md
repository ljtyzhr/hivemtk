# Phase 1 Quickstart: 双形态端到端验证指南

**Feature**: 001-saas-multi-tenant | **Date**: 2026-10-17

前置：Docker 环境；`make install` 后按 `.env` 配置。测试库连接变量导出见 CLAUDE.md §3.5（`POSTGRES_TEST_*`）。

## 场景 A：离线模式零回归（存量用户视角）

1. 用存量数据库（含历史数据）启动，`.env` 不设 `TENANT_MODE`（默认 off）。
2. 登录工作台 → 抽查客户/会话/SOP/知识库/报表各 1 页，数据与升级前一致。
3. 运行全量既有测试：`make test-go`（off 模式语义下全部通过，零测试改动）。
4. 访问 `POST /api/tenant/register` → 返回业务码 4104（注册未开放）。
5. 后台任务跑一个周期（SOP/定时/轮询），日志无租户相关报错，产出正常。

**预期**：行为与升级前完全一致；DB 中各业务表 tenant_id=1。

## 场景 B：SaaS 模式开户与隔离

1. `.env` 设 `TENANT_MODE=on` 启动。
2. 注册页开户租户 A（企业名+管理员+密码）→ 自动登录，工作台可用；DB 确认 tenants 新行 + 管理员账号带租户归属。
3. 同法开户租户 B；两租户各录入客户/渠道/知识库标记数据。
4. 租户 A 登录态：列表只含 A 数据；用 B 的资源 ID 直查/改/删 → 404/403；导出仅含 A。
5. 伪造无租户 claim 的 token 访问受保护 API → 401。

## 场景 C：渠道入站租户解析

1. 租户 A 配置 Telegram 渠道账号。
2. 向该 bot 发消息 → 会话/客户/自动回复归属租户 A。
3. 伪造不存在 account_ref 的 webhook → 404 + 安全告警日志。

## 场景 D：LLM 双轨

1. 新租户零配置发起 AI 会话 → 平台池响应成功。
2. 租户配置自有模型地址（指向可观测的 mock 服务）→ 会话请求命中 mock；平台池零调用。
3. 删除配置 → 回退平台池；另一租户不受影响。

## 场景 E：后台任务扇出

1. 两租户各建延时任务（SOP 定时/延时触达）。
2. 等待执行窗口 → 两租户任务各自执行、各自租户上下文产出正确（收件人/文案/日志归属）。
3. off 模式回归场景 A 第 5 步确认退化等价。

## 场景 F：CI 门禁

1. `scripts/check-gorm-only.sh` 通过（业务包 0 处 `.Exec(`/`.Raw(`）。
2. 故意在业务 repo 加一处 `.Raw(` → 门禁失败（反向验证）。

## 验收基线

- `make audit` + `make lint` + `make vet` + `make test-go` 全绿。
- 越权测试套件（两租户交叉全 API 域扫描）0 泄漏。
- 详细端点契约见 [contracts/api.md](./contracts/api.md)；表分类见 [data-model.md](./data-model.md)。

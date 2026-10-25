# 密钥文件挂载与轮换演练（OPT-SEC-08）

> 2026-10-24 补齐。对应原《未完成事项实施规划-2026-10-23》§4.3 E-3（该文档已归档，结论并入 `docs/任务总表-2026-10-24.md`）。
> 实现通道：`user-server/internal/secrets/fileloader.go`（`LoadFiles`），启动预加载在
> `cmd/api/main.go`（`HIVEMTK_SECRETS_DIR`，默认 `/run/secrets`）。
> **决策口径（规划推荐）**：文件挂载是一等方案，不接 Vault/KMS SDK；只有需要动态租约
> （数据库短时凭证）时才引 SDK。

## 1. 运行约定

- 启动顺序：`LoadFiles(/run/secrets)` → `secrets.InitFromEnv()`。
- **env 优先**：进程里已显式设置的同名变量不会被文件覆盖（`docker run -e` / compose
  `environment:` 覆盖挂载文件；也便于本地调试）。
- 目录不存在 ⇒ 视为未启用，静默跳过（不报错、不告警）。
- 单文件解析失败 ⇒ 跳过该文件继续，聚合报 WARN，**不中断启动**。
- `MASTER_KEY` 缺失时非 dev 姿态直接 `os.Exit(1)`（见 `main.go`）；文件通道注入成功即可过检。

### 文件风格（二选一）

| 风格 | 文件 | 变量名 | 值 |
|---|---|---|---|
| 单值 | `/run/secrets/MASTER_KEY.txt` | `MASTER_KEY` | 文件**首行**内容（去首尾空白） |
| env | `/run/secrets/app_secrets.env` | 逐行 KEY=VALUE | `#` 注释；值去首尾空白 |

生产常用密钥：`MASTER_KEY`（32+ 字节）、`POSTGRES_PASSWORD`、`USER_JWT_SECRET`（或 `JWT_SECRET`）、`MERCHANT_HMAC_SECRET`。

## 2. 部署示例

### Docker Compose

```yaml
services:
  user-server:
    secrets:
      - master_key
      - postgres_password
    environment:
      # 可选：自定义目录（默认 /run/secrets）
      # HIVEMTK_SECRETS_DIR: /run/secrets
      APP_ENV: production
secrets:
  master_key:
    file: ./secrets/MASTER_KEY.txt
  postgres_password:
    file: ./secrets/POSTGRES_PASSWORD.txt
```

### Kubernetes

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: hivemtk-secrets
type: Opaque
stringData:
  MASTER_KEY.txt: "<32+字节密钥>"
  POSTGRES_PASSWORD.txt: "<db 口令>"
---
# Deployment 里挂载：
# volumeMounts:
#   - name: secrets
#     mountPath: /run/secrets
#     readOnly: true
# volumes:
#   - name: secrets
#     secret: { secretName: hivemtk-secrets }
```

Vault Agent templating 输出到同一目录即兼容（模板渲染 `*_txt` 文件）。

## 3. 轮换演练（验收要求：换文件不改代码/不重启逻辑，服务读到新值）

轮换是**重启级**动作（env 在进程启动时读取一次），但**零代码/零配置变更**：

1. 写入新值：更新 K8s Secret 或覆盖 `/run/secrets/MASTER_KEY.txt` 内容。
2. 滚动重启容器（`kubectl rollout restart deploy/user-server` / `docker compose up -d --force-recreate user-server`）。
3. 验证：日志出现 `[secrets] 从 /run/secrets 注入 N 个密钥环境变量` 且服务健康检查通过。

### 已工具化的本机轮换

`hivemtk/scripts/rotate-secrets.sh`（`.env` 四仓 + ALTER USER + 双服务重启一体化；
授权位 `ROTATE_AUTHORIZED=1`，见脚本头注的 F1 决策链）。文件挂载通道与它并行：
`.env` 管"本机开发/测试"，`/run/secrets` 管"容器编排"。

### env 覆盖文件行为的测试锚点

`internal/secrets/fileloader_test.go`：已存在的 env 不被文件覆盖、目录不存在静默跳过、
坏文件聚合报错不中断。改 `LoadFiles` 语义先跑这三条。

## 4. 何时才引 Vault SDK

出现以下**任一**再评估（规划原文口径）：

1. 数据库要求短时动态凭证（租约 < 1h，人手轮换不可行）；
2. 合规审计要求密钥访问留痕与租约回收；
3. 多集群密钥分发需要统一控制面。

届时接 Vault Agent 的 database secrets engine（短时凭证仍以文件/env 形态落到容器），
应用代码（`LoadFiles`/`InitFromEnv`）不变。

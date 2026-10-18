# HiveMtk 文档索引

> 单商户本地部署（私域模式）
> 三份主文档回答三个问题：**有哪些功能**（官方文档）· **如何安装运维**（运维手册）· **出问题怎么办**（排查手册）

---

## 一、快速开始（三份主文档）

| 文档 | 回答的问题 |
|------|------|
| [README.md](../README.md) | 仓库入口、目录结构、快速开始 |
| [marketing-features/README.md](marketing-features/README.md) | 官方文档：产品有哪些功能（含已知限制如实披露） |
| [DEPLOYMENT_GUIDE.md](DEPLOYMENT_GUIDE.md) | 运维手册：如何安装、日常如何运维（端口/命令以源码为准） |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | 排查手册：用户会遇到的问题，按现象组织，快速索引表入口 |

---

## 二、功能文档 · 操作手册（handbook）

| 文档 | 读者 |
|------|------|
| [handbook/README.md](handbook/README.md) | 四册入口索引 |
| [handbook/FEATURES-FOR-HUMANS.md](handbook/FEATURES-FOR-HUMANS.md) | 功能全集 · 渠道能力 · AI 引擎 · 已知限制（人读） |
| [handbook/OPERATIONS-FOR-HUMANS.md](handbook/OPERATIONS-FOR-HUMANS.md) | 部署 · 配置 · 日常运营 · 备份恢复（人读） |
| [handbook/FEATURES-FOR-AI.md](handbook/FEATURES-FOR-AI.md) | 路由表 · 模块地图 · 文件位置 · 数据结构（AI 读） |
| [handbook/OPERATIONS-FOR-AI.md](handbook/OPERATIONS-FOR-AI.md) | 分层铁律 · 命令 · 门禁 · 新增功能流程（AI 读） |

---

## 三、部署与运维

| 文档 | 描述 |
|------|------|
| [RELEASE_PREPARATION.md](RELEASE_PREPARATION.md) | 发布准备：用户端/平台端配置、数据初始化、安装部署逻辑核查 + 上服务器清单 |
| [operations/MERCHANT_DEPLOYMENT.md](operations/MERCHANT_DEPLOYMENT.md) | 商户部署手册 |
| [operations/HA_DEPLOYMENT.md](operations/HA_DEPLOYMENT.md) | 高可用部署（单机+备份） |
| [operations/DR_RECOVERY.md](operations/DR_RECOVERY.md) | 灾难恢复方案 |
| [operations/SLA_SLO.md](operations/SLA_SLO.md) | 服务等级承诺 |
| [operations/secret_rotation.md](operations/secret_rotation.md) | 密钥轮换指南 |

---

## 四、架构

| 文档 | 描述 |
|------|------|
| [architecture/部署方案_用户端.md](architecture/部署方案_用户端.md) | 用户端部署架构 |
| [architecture/HOST_INFERENCE_PLAN.md](architecture/HOST_INFERENCE_PLAN.md) | llama.cpp 推理方案 |
| [architecture/adr/](architecture/adr/) | 架构决策记录 |

---

## 五、核心功能

| 文档 | 描述 |
|------|------|
| [marketing-features/README.md](marketing-features/README.md) | 核心功能总览（同快速开始） |
| [operations/AI_AGENT_PERF_DEPLOY.md](operations/AI_AGENT_PERF_DEPLOY.md) | AI 销冠部署 |
| [operations/AI_AGENT_PERF_API.md](operations/AI_AGENT_PERF_API.md) | AI 销冠 API |
| [operations/KNOWLEDGE_GROUP_DEPLOY.md](operations/KNOWLEDGE_GROUP_DEPLOY.md) | 知识库部署 |
| [operations/PERFORMANCE_BENCHMARK.md](operations/PERFORMANCE_BENCHMARK.md) | 性能基准测试 |

---

## 六、项目规则

| 文档 | 描述 |
|------|------|
| [standards/MASTER_RULES.md](standards/MASTER_RULES.md) | 项目核心规则与编码规范 |

---

## 七、嵌入 SDK 与反向代理

| 文档 | 描述 |
|------|------|
| [operations/CHAT_WIDGET_EMBED.md](operations/CHAT_WIDGET_EMBED.md) | Chat Widget 嵌入 SDK 接入指南（`embed-sdk`，ADR-011） |
| [architecture/adr/ADR-011-chat-widget-embed.md](architecture/adr/ADR-011-chat-widget-embed.md) | ADR-011：Chat Widget 嵌入方案决策记录 |
| [architecture/FRP私域部署指南.md](architecture/FRP私域部署指南.md) | FRP 内网穿透私域部署指南 |
| [operations/reverse-proxy/frpc.toml.template](operations/reverse-proxy/frpc.toml.template) | frpc 客户端配置模板 |

---

## 八、相关资源

- [ADR 决策记录](architecture/adr/) — 架构决策历史
- [Bridge 运行手册](operations/Bridge_Runbook.md) — Bridge 桥接模块（内部设计稿 docs/bridge/ 按 .gitignore 只留本地，不入库）
- [AI 功能清单基线](architecture/AI_CORE_FEATURE_INVENTORY.md) — AI 核心链路功能点 F1-F15 / 短板 G1-G12 事实来源（源码实测）

---

*最后更新: 2026-08-27 · 三份主文档已按源码完成事实校准；AI 功能清单基线已收编入仓库*

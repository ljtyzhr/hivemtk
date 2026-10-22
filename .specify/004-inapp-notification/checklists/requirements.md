# Specification Quality Checklist: 站内消息通知模块

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-10-22
**Feature**: [../spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

### 第 1 轮校验（2026-10-22）

校验中发现并已修复的问题：

1. **「平台消息」一词在 spec 中有两种读法** —— 既可指"平台上发布的那条"，
   也可指"送达用户端后的副本"。已在 Key Entities 中拆成两个独立实体
   （平台消息 / 通知），并在 FR-006/FR-007 中明确二者的关系
   （通知携带来源平台消息标识，用于幂等去重）。
2. **已读语义的归属方未言明** —— 原文只说"标记已读"，未说明是标记在消息上
   还是标记在人与消息的关系上。已在 FR-012/FR-014 明确为"按用户独立记录"，
   并在 Assumptions 中写明本地业务通知沿用旧语义、本次不改动。

### 关于"无实现细节"的说明

spec 正文刻意不出现 Go / GORM / Vue / 表名 / 端点路径等实现词，
需求以"系统 MUST …"的外部可观测行为表述。
但 Assumptions 一节必须交代若干**部署形态事实**（拉取式投递、3 分钟周期、
租户短码作为对接标识）—— 这些是**既有的、可被代码验证的事实**，
不是对未来的实现指令；省略它们会让交付范围出现不可判定的空洞，
故予以保留并明确标注为"假设"。

### 范围边界

本次明确 **不做** 的部分（见 Assumptions 末条及 spec 目录外说明）：
移动端通知展示、消息富文本、平台主动回调推送、本地业务通知的已读语义迁移。
这些不做项若后续要纳入，须另开 spec 周期。

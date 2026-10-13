# 缺失的配置门脚本 — 评估

## 现状
文档 §6 提到 5 个文件未跟踪且被 Makefile+CI 直接引用：
- `scripts/check-text-encoding.py`
- `scripts/text-encoding.baseline`
- `scripts/check-text-encoding.test.sh`
- `scripts/config-param-defaults.baseline`
- `scripts/measure-tunable-blindspot.py`

## 检查结果
- 工作目录中不存在（`ls scripts/check-text-encoding*` → no matches found）
- git 历史中不存在（`git log --all` 查询返回空）
- `scripts/` 目录下无任何 `check-*` 脚本

## 结论
这些文件从未被创建或已被删除，无法自动恢复。需要用户提供来源或内容。

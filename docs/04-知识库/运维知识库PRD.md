# 运维知识库 PRD（v0.1）

- 文档状态：Draft
- 版本：v0.1
- 适用项目：GWF（Go Watch File）
- 更新时间：2026-02-17
- 负责人：平台研发 / 运维团队

## 1. 背景与目标

GWF 当前已具备文件监控上传、告警决策、AI 日志分析、运维控制台等能力，但运维知识仍分散在 `docs/`、FAQ、告警规则、日志分析记录中，导致重复排障和经验流失。

本需求新增“运维知识库”模块，形成闭环：

`告警/日志 -> AI 分析 -> 人工确认 -> 知识沉淀 -> 检索/问答 -> 处置复用`

### 1.1 业务目标

1. 降低重复故障分析成本，缩短 MTTD/MTTR。
2. 将个人经验沉淀为可检索、可审核、可追溯的标准化知识。
3. 为告警处置和 AI 分析提供“有依据”的推荐与回答。

### 1.2 北极星指标与量化目标

1. 知识检索命中率：20 条典型问题命中率 >= 80%。
2. AI 回答引用率：知识问答返回引用来源覆盖率 = 100%。
3. 故障定位效率：常见告警场景 MTTD 下降 >= 30%。
4. 知识治理完整性：知识条目版本追踪与回滚可用率 = 100%。

## 2. 范围定义

### 2.1 MVP（本期必须完成）

1. 知识条目管理：创建、编辑、发布、归档。
2. 知识分类与标签：支持按系统、模块、严重级别、场景打标签。
3. 知识导入：支持从 Markdown 文档和 FAQ 批量导入。
4. 知识检索：关键词检索 + 标签过滤 + 更新时间排序。
5. 知识问答：基于检索结果生成回答，强制返回引用条目。
6. 告警联动推荐：在告警详情给出关联知识条目与建议动作。
7. 变更追溯：支持版本记录、发布记录、回滚到历史版本。

### 2.2 明确不做（MVP 外）

1. 多租户和细粒度 RBAC。
2. 无审批全自动处置（Auto-remediation 全放开）。
3. 复杂跨系统工作流编排。
4. 大规模报表平台与计费体系。

## 3. 用户角色与权限（MVP）

1. 运维工程师：查询知识、提问、创建草稿、提交更新。
2. 值班负责人：审核并发布知识、归档过期知识、触发回滚。
3. 只读访客（可选）：仅查看已发布知识，不可编辑。

MVP 权限策略复用现有 API Token 机制，先采用“单 Token + 操作审计日志”的轻量模式。

## 4. 核心用户故事

1. 作为值班工程师，我希望在收到告警后，系统自动推荐相关 运行手册，减少人工检索时间。
2. 作为运维工程师，我希望把一次有效排障过程沉淀为知识条目，供后续复用。
3. 作为审核人，我希望看到条目变更 diff 与影响范围后再发布，降低错误知识扩散风险。
4. 作为使用者，我希望 AI 回答必须带出处，便于快速核验真实性。

## 5. 业务流程

### 5.1 知识入库流程

1. 录入来源：手工创建或导入 `docs/*.md` / FAQ。
2. 结构化处理：抽取标题、摘要、标签、关键字、适用模块。
3. 存储：保存条目主体、版本快照、来源映射、状态（draft/published/archived）。
4. 发布：审核通过后切换为 `published`，可被检索与问答引用。

### 5.2 检索与问答流程

1. 用户输入关键词或问题。
2. 系统检索 TopK 知识条目（关键词 + 标签 + 时间加权）。
3. 问答模块仅基于 TopK 内容生成答案。
4. 响应返回：答案 + 引用条目 + 置信提示。

### 5.3 告警联动流程

1. 告警模块产生告警决策（现有 `/api/alerts` 数据）。
2. 解析告警关键信息（规则名、严重级别、关键错误词）。
3. 调用知识检索返回推荐条目（最多 3 条）。
4. 前端告警详情展示“推荐知识”和“建议动作”。

## 6. 信息架构与页面改造

基于现有 `console-frontend` 的三控制台结构，在侧边栏新增一级视图：`知识库控制台`。

### 6.1 页面清单

1. 知识列表页：搜索、筛选、状态切换、排序。
2. 知识详情页：正文、元信息、引用来源、版本时间线。
3. 编辑页：Markdown 编辑、标签维护、预览、提交审核。
4. 审核页：待审核列表、diff 对比、发布/驳回。
5. 告警详情增强区：推荐知识卡片与跳转入口。

### 6.2 关键交互要求

1. 列表默认只显示 `published` 条目。
2. AI 问答响应区必须显示引用来源（至少 1 条）。
3. 归档条目默认不参与检索，除非显式勾选“含归档”。
4. 每次发布必须记录发布人、发布时间、版本号与变更说明。

## 7. 技术架构（贴合当前项目）

### 7.1 架构原则

1. 复用现有 Go API 服务，不引入独立微服务。
2. 保持单机可部署，支持后续扩展多 Agent。
3. 与现有 Token 鉴权、CORS、运行时配置机制兼容。

### 7.2 组件设计

1. `KB API`（新增）：条目管理、检索、问答、版本、审核接口。
2. `KB Store`（新增）：SQLite（含 FTS）存储元数据与全文索引。
3. `KB Content Store`（新增）：知识正文文件（Markdown）和版本快照（本地磁盘）。
4. `KB RAG Engine`（新增）：检索 TopK + 调用现有 AI 接口风格生成回答。
5. `Alert Adapter`（新增）：把告警字段映射为知识检索条件。

### 7.3 与现有模块集成点

1. 后端入口：`go-watch-file/internal/api/server.go` 新增 `/api/kb/*` 路由。
2. AI 能力：复用 `internal/api/ai_log_summary.go` 的超时、降级、解析策略。
3. 配置持久化：在 `config.runtime.yaml` 增加知识库开关与路径配置。
4. 前端入口：`console-frontend/src/App.tsx` 与侧边栏新增 `knowledge` 视图。

## 8. 数据模型（MVP）

### 8.1 实体定义

1. `kb_articles`
   - `id`（string）
   - `title`（string）
   - `summary`（string）
   - `category`（string）
   - `severity`（low/medium/high）
   - `status`（draft/published/archived）
   - `current_version`（int）
   - `created_by`（string）
   - `updated_by`（string）
   - `created_at`（datetime）
   - `updated_at`（datetime）

2. `kb_article_versions`
   - `id`（string）
   - `article_id`（string）
   - `version`（int）
   - `content_markdown`（text）
   - `change_note`（string）
   - `source_type`（manual/import/ai-generated）
   - `source_ref`（string）
   - `created_by`（string）
   - `created_at`（datetime）

3. `kb_tags`
   - `id`（string）
   - `name`（string）
   - `type`（system/module/scenario/custom）

4. `kb_article_tags`
   - `article_id`（string）
   - `tag_id`（string）

5. `kb_reviews`
   - `id`（string）
   - `article_id`（string）
   - `target_version`（int）
   - `action`（submit/approve/reject/archive/rollback）
   - `comment`（string）
   - `operator`（string）
   - `created_at`（datetime）

6. `kb_references`
   - `id`（string）
   - `article_id`（string）
   - `ref_type`（doc/faq/alert/ai-summary/external）
   - `ref_path`（string）
   - `ref_title`（string）

### 8.2 检索索引

1. 标题 + 摘要 + 正文建立 FTS 索引。
2. 标签、状态、更新时间建立过滤与排序索引。

## 9. API 设计（MVP）

所有接口沿用现有鉴权规则：`Authorization: Bearer <token>` 或 `X-API-Token`。

### 9.1 知识管理

1. `POST /api/kb/articles`
   - 创建草稿条目。
2. `PUT /api/kb/articles/{id}`
   - 更新草稿并生成新版本。
3. `GET /api/kb/articles/{id}`
   - 获取条目详情（含当前版本）。
4. `GET /api/kb/articles`
   - 列表检索（q/tags/status/severity/page/pageSize/sort）。
5. `POST /api/kb/articles/{id}/submit`
   - 提交审核。
6. `POST /api/kb/articles/{id}/approve`
   - 审核发布。
7. `POST /api/kb/articles/{id}/reject`
   - 驳回并附备注。
8. `POST /api/kb/articles/{id}/archive`
   - 归档。
9. `POST /api/kb/articles/{id}/rollback`
   - 回滚到指定版本。

### 9.2 导入与检索

1. `POST /api/kb/import/docs`
   - 批量导入 `docs/` 与 FAQ 文档。
2. `POST /api/kb/search`
   - 返回匹配条目与摘要片段。

### 9.3 问答与联动

1. `POST /api/kb/ask`
   - 输入问题，返回 `answer + citations + confidence`。
2. `GET /api/kb/recommendations?alertId=...`
   - 根据告警返回推荐知识条目。

### 9.4 示例响应（`/api/kb/ask`）

```json
{
  "ok": true,
  "answer": "当前更可能是上传队列拥塞与重试退避不足导致的延迟堆积，建议先扩容 workers 并检查失败分类。",
  "citations": [
    {
      "articleId": "kb_001",
      "title": "上传队列拥塞排障手册",
      "version": 3
    }
  ],
  "confidence": 0.82
}
```

## 10. 非功能要求

1. 安全性：复用现有 Token 鉴权，写操作全量审计。
2. 性能：知识列表 P95 < 300ms，问答接口 P95 < 5s（不含外部模型波动）。
3. 可用性：AI 不可用时，检索服务仍可用并提示“问答降级”。
4. 可观测性：增加指标 `kb_search_total`、`kb_search_hit_ratio`、`kb_ask_total`、`kb_ask_citation_ratio`、`kb_review_latency_ms`。

## 11. 迭代计划（建议 3 个迭代）

### 迭代 1（1~2 周）

1. 完成数据表与 CRUD API。
2. 支持 Markdown 手工录入与发布流。
3. 前端完成知识列表/详情/编辑页面。
4. 验收：可创建-审核-发布-检索闭环。

### 迭代 2（1~2 周）

1. 完成 docs/FAQ 导入器。
2. 完成 FTS 检索与标签过滤。
3. 完成版本 diff 与回滚能力。
4. 验收：典型问题检索命中率达到 70%+。

### 迭代 3（1~2 周）

1. 完成 `/api/kb/ask`（RAG + 引用强约束）。
2. 接入告警推荐 `/api/kb/recommendations`。
3. 控制台联动告警详情展示推荐知识。
4. 验收：问答引用率 100%，告警联动可用。

## 12. 测试与发布门禁

1. 后端单测：模型、检索、审核流、回滚流、问答引用约束。
2. 前端构建：`npm run build` 必须通过。
3. 集成测试：导入 -> 发布 -> 检索 -> 告警推荐全链路通过。
4. 回归重点：不得影响现有 `/api/dashboard`、`/api/alerts`、`/api/ai/log-summary` 行为。

## 13. 风险与回滚

1. AI 幻觉风险：问答必须引用来源；无引用则返回失败，不给“猜测答案”。
2. 知识过期风险：新增有效期与定期复审任务，过期自动标记待复审。
3. 错误发布风险：发布前强制审核，支持一键回滚到历史版本。
4. 性能风险：问答与检索分层降级，AI 超时不阻塞检索接口。
5. 数据损坏风险：知识库文件和 SQLite 定时备份，支持冷恢复。

## 14. 验收口径（DoD）

1. 代码可构建，新增 API 有测试覆盖。
2. 知识条目支持完整生命周期（草稿/发布/归档/回滚）。
3. 告警详情可看到知识推荐并可跳转详情。
4. AI 问答响应始终携带至少 1 条引用。
5. 文档同步完成：README 文档入口 + 开发指南补充知识库章节。


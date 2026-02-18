/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于知识库控制台页面 负责条目管理 审核与问答交互 */

import { useEffect, useMemo, useState, type ReactNode } from "react";
import "./KnowledgeConsole.css";
import type { KnowledgeArticle, KnowledgeAskResponse } from "./types";
import {
  fetchKBArticle,
  fetchKBArticles,
  fetchKBMetrics,
  fetchKBPendingReviews,
  postKBArticle,
  postKBArticleAction,
  postKBAsk,
  postKBImportDocs,
  postKBRollback,
  postKBSearch,
  putKBArticle,
  type KnowledgeMetricsSnapshot,
} from "./console/dashboardApi";

type Severity = "low" | "medium" | "high";
type ArticleStatusFilter = "all" | "draft" | "published" | "archived";

type ArticleForm = {
  title: string;
  summary: string;
  category: string;
  severity: Severity;
  tags: string;
  content: string;
  changeNote: string;
};

const emptyForm: ArticleForm = {
  title: "",
  summary: "",
  category: "runbook",
  severity: "medium",
  tags: "",
  content: "",
  changeNote: "",
};

const parseTags = (raw: string) =>
  raw
    .split(/[,\s，；;]+/)
    .map((item) => item.trim())
    .filter(Boolean);

const toTagText = (tags: string[] | undefined) => (tags?.length ? tags.join(", ") : "");

const formatRatio = (value: number | null) => {
  if (value === null) return "--";
  return `${(value * 100).toFixed(1)}%`;
};

const formatLatencyMs = (value: number | null) => {
  if (value === null) return "--";
  return `${Math.round(value)} ms`;
};

type EditorMode = "edit" | "preview" | "diff";

type DiffLine = {
  kind: "same" | "added" | "removed" | "changed-left" | "changed-right";
  text: string;
};

const splitLines = (value: string) => value.replace(/\r\n/g, "\n").split("\n");

// buildLineDiff 用于编辑态与历史版本的行级对比
// 采用轻量逐行比较 目标是快速定位改动而不是实现完整 diff 算法
const buildLineDiff = (base: string, current: string): DiffLine[] => {
  const baseLines = splitLines(base);
  const currentLines = splitLines(current);
  const max = Math.max(baseLines.length, currentLines.length);
  const out: DiffLine[] = [];
  for (let i = 0; i < max; i++) {
    const left = baseLines[i] ?? "";
    const right = currentLines[i] ?? "";
    if (left === right) {
      out.push({ kind: "same", text: right });
      continue;
    }
    if (!left && right) {
      out.push({ kind: "added", text: right });
      continue;
    }
    if (left && !right) {
      out.push({ kind: "removed", text: left });
      continue;
    }
    out.push({ kind: "changed-left", text: left });
    out.push({ kind: "changed-right", text: right });
  }
  return out;
};

// renderMarkdownPreview 提供受控的 Markdown 子集渲染
// 只支持常用标题 列表 代码块 避免引入复杂解析器带来的安全和维护成本
const renderMarkdownPreview = (content: string): ReactNode => {
  const lines = splitLines(content);
  const nodes: ReactNode[] = [];
  let listBuffer: string[] = [];
  let codeBuffer: string[] = [];
  let inCode = false;

  const flushList = () => {
    if (listBuffer.length === 0) return;
    nodes.push(
      <ul className="kb-md-list" key={`list-${nodes.length}`}>
        {listBuffer.map((item, idx) => (
          <li key={`${item}-${idx}`}>{item}</li>
        ))}
      </ul>
    );
    listBuffer = [];
  };

  const flushCode = () => {
    if (codeBuffer.length === 0) return;
    nodes.push(
      <pre className="kb-md-code" key={`code-${nodes.length}`}>
        <code>{codeBuffer.join("\n")}</code>
      </pre>
    );
    codeBuffer = [];
  };

  for (const raw of lines) {
    const line = raw.trimEnd();
    if (line.startsWith("```")) {
      if (inCode) {
        flushCode();
        inCode = false;
      } else {
        flushList();
        inCode = true;
      }
      continue;
    }
    if (inCode) {
      codeBuffer.push(raw);
      continue;
    }
    const trimmed = line.trim();
    if (trimmed === "") {
      flushList();
      continue;
    }
    if (trimmed.startsWith("- ") || trimmed.startsWith("* ")) {
      listBuffer.push(trimmed.slice(2).trim());
      continue;
    }
    flushList();
    if (trimmed.startsWith("### ")) {
      nodes.push(<h4 key={`h4-${nodes.length}`}>{trimmed.slice(4).trim()}</h4>);
      continue;
    }
    if (trimmed.startsWith("## ")) {
      nodes.push(<h3 key={`h3-${nodes.length}`}>{trimmed.slice(3).trim()}</h3>);
      continue;
    }
    if (trimmed.startsWith("# ")) {
      nodes.push(<h2 key={`h2-${nodes.length}`}>{trimmed.slice(2).trim()}</h2>);
      continue;
    }
    nodes.push(
      <p className="kb-md-paragraph" key={`p-${nodes.length}`}>
        {trimmed}
      </p>
    );
  }
  flushList();
  flushCode();
  if (nodes.length === 0) {
    return <div className="empty-state">暂无可预览内容</div>;
  }
  return <div className="kb-md-preview">{nodes}</div>;
};

// KnowledgeConsole 承载知识库检索 编辑 审核和问答流程
// 页面通过局部 loading 状态拆分 避免一次操作阻塞全部交互
export function KnowledgeConsole() {
  const [query, setQuery] = useState("");
  const [statusFilter, setStatusFilter] = useState<ArticleStatusFilter>("published");
  const [severityFilter, setSeverityFilter] = useState<"all" | Severity>("all");
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [importing, setImporting] = useState(false);
  const [asking, setAsking] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const [articles, setArticles] = useState<KnowledgeArticle[]>([]);
  const [total, setTotal] = useState(0);
  const [selectedID, setSelectedID] = useState("");
  const [selectedDetail, setSelectedDetail] = useState<KnowledgeArticle | null>(null);
  const [form, setForm] = useState<ArticleForm>(emptyForm);
  const [editorMode, setEditorMode] = useState<EditorMode>("edit");
  const [askQuestion, setAskQuestion] = useState("");
  const [askResult, setAskResult] = useState<KnowledgeAskResponse | null>(null);
  const [rollbackVersion, setRollbackVersion] = useState("");
  const [pendingReviews, setPendingReviews] = useState<KnowledgeArticle[]>([]);
  const [pendingLoading, setPendingLoading] = useState(false);
  const [kbMetrics, setKBMetrics] = useState<KnowledgeMetricsSnapshot | null>(null);
  const [metricsLoading, setMetricsLoading] = useState(false);

  const selectedArticle = useMemo(
    () => articles.find((item) => item.id === selectedID) ?? null,
    [articles, selectedID]
  );
  const previousVersionContent = useMemo(() => {
    const versions = selectedDetail?.versions ?? [];
    if (versions.length < 2) return "";
    const sorted = [...versions].sort((a, b) => b.version - a.version);
    return sorted[1]?.content ?? "";
  }, [selectedDetail]);
  const diffLines = useMemo(
    () => buildLineDiff(previousVersionContent, form.content),
    [previousVersionContent, form.content]
  );
  const searchHitRatio = kbMetrics?.searchHitRatio ?? null;
  const askCitationRatio = kbMetrics?.askCitationRatio ?? null;
  const reviewLatencyP95 = kbMetrics?.reviewLatencyP95Ms ?? null;

  // loadArticles 是列表主刷新函数
  // 所有筛选条件最终都汇聚到这里 便于统一维护请求参数口径
  const loadArticles = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await fetchKBArticles({
        q: query.trim() || undefined,
        status: statusFilter === "all" ? undefined : statusFilter,
        severity: severityFilter === "all" ? undefined : severityFilter,
        page: 1,
        pageSize: 50,
        includeArchived: statusFilter === "all",
      });
      setArticles(data.items ?? []);
      setTotal(data.total ?? 0);
      const hasSelected = data.items?.some((item) => item.id === selectedID);
      if (!hasSelected) {
        setSelectedID(data.items?.[0]?.id ?? "");
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  const loadArticleDetail = async (articleID: string) => {
    if (!articleID) {
      setForm(emptyForm);
      return;
    }
    setError(null);
    try {
      const data = await fetchKBArticle(articleID);
      const article = data.article;
      setSelectedDetail(article);
      setForm({
        title: article.title ?? "",
        summary: article.summary ?? "",
        category: article.category ?? "runbook",
        severity: article.severity ?? "medium",
        tags: toTagText(article.tags),
        content: article.content ?? "",
        changeNote: "",
      });
    } catch (err) {
      setError((err as Error).message);
    }
  };

  const loadPendingReviews = async () => {
    setPendingLoading(true);
    try {
      const data = await fetchKBPendingReviews(20);
      setPendingReviews(data.items ?? []);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setPendingLoading(false);
    }
  };

  const loadKBMetrics = async () => {
    setMetricsLoading(true);
    try {
      const snapshot = await fetchKBMetrics();
      setKBMetrics(snapshot);
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setMetricsLoading(false);
    }
  };

  useEffect(() => {
    void loadArticles();
  }, [query, statusFilter, severityFilter]);

  useEffect(() => {
    void loadPendingReviews();
  }, []);

  useEffect(() => {
    void loadKBMetrics();
  }, []);

  useEffect(() => {
    if (!selectedID) {
      setSelectedDetail(null);
      setForm(emptyForm);
      return;
    }
    setEditorMode("edit");
    void loadArticleDetail(selectedID);
  }, [selectedID]);

  const handleCreate = async () => {
    if (!form.title.trim()) {
      setError("标题不能为空");
      return;
    }
    if (!form.content.trim()) {
      setError("内容不能为空");
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      const data = await postKBArticle({
        title: form.title.trim(),
        summary: form.summary.trim(),
        category: form.category.trim() || "runbook",
        severity: form.severity,
        content: form.content,
        tags: parseTags(form.tags),
        changeNote: form.changeNote.trim() || "create from console",
        createdBy: "console",
      });
      setMessage("已创建知识条目");
      setSelectedID(data.article.id);
      await loadArticles();
      await loadPendingReviews();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleSave = async () => {
    if (!selectedID) {
      setError("请先选择一个条目");
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await putKBArticle(selectedID, {
        title: form.title.trim(),
        summary: form.summary.trim(),
        category: form.category.trim(),
        severity: form.severity,
        content: form.content,
        tags: parseTags(form.tags),
        changeNote: form.changeNote.trim() || "update from console",
        updatedBy: "console",
      });
      setMessage("已保存并生成新版本");
      await loadArticles();
      await loadArticleDetail(selectedID);
      await loadPendingReviews();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleAction = async (action: "submit" | "approve" | "reject" | "archive") => {
    if (!selectedID) {
      setError("请先选择一个条目");
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await postKBArticleAction(selectedID, action, {
        operator: "console",
        comment: `${action} from console`,
      });
      setMessage(`状态已更新：${action}`);
      await loadArticles();
      await loadArticleDetail(selectedID);
      await loadPendingReviews();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleRollback = async () => {
    if (!selectedID) {
      setError("请先选择一个条目");
      return;
    }
    const targetVersion = Number.parseInt(rollbackVersion, 10);
    if (!Number.isFinite(targetVersion) || targetVersion <= 0) {
      setError("请输入有效的版本号");
      return;
    }
    setSaving(true);
    setError(null);
    setMessage(null);
    try {
      await postKBRollback(selectedID, {
        targetVersion,
        operator: "console",
        comment: `rollback to ${targetVersion}`,
      });
      setMessage(`已回滚到版本 ${targetVersion}`);
      await loadArticles();
      await loadArticleDetail(selectedID);
      await loadPendingReviews();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const handleImportDocs = async () => {
    setImporting(true);
    setError(null);
    setMessage(null);
    try {
      const data = await postKBImportDocs({ path: "docs", operator: "console" });
      setMessage(`导入完成：新增 ${data.result.imported}，更新 ${data.result.updated}，跳过 ${data.result.skipped}`);
      await loadArticles();
      await loadPendingReviews();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setImporting(false);
    }
  };

  const handleAsk = async () => {
    if (!askQuestion.trim()) {
      setError("请输入问题");
      return;
    }
    setAsking(true);
    setError(null);
    try {
      const data = await postKBAsk({ question: askQuestion.trim(), limit: 3 });
      setAskResult(data);
      await loadKBMetrics();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setAsking(false);
    }
  };

  const handleSemanticSearch = async () => {
    if (!query.trim()) {
      setError("请输入检索关键词");
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const data = await postKBSearch({ query: query.trim(), limit: 50, includeArchived: statusFilter === "all" });
      setArticles(data.items ?? []);
      setTotal(data.items?.length ?? 0);
      setSelectedID(data.items?.[0]?.id ?? "");
      await loadKBMetrics();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="knowledge-shell">
      <section className="panel knowledge-header">
        <div className="section-title">
          <h2>运维知识库控制台</h2>
          <span>条目总数 {total}</span>
        </div>
        <div className="knowledge-toolbar">
          <input
            className="search"
            placeholder="搜索标题/摘要/内容"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <select
            className="select"
            value={statusFilter}
            onChange={(e) => setStatusFilter(e.target.value as ArticleStatusFilter)}
          >
            <option value="all">全部状态</option>
            <option value="draft">草稿</option>
            <option value="published">已发布</option>
            <option value="archived">已归档</option>
          </select>
          <select
            className="select"
            value={severityFilter}
            onChange={(e) => setSeverityFilter(e.target.value as "all" | Severity)}
          >
            <option value="all">全部级别</option>
            <option value="low">低</option>
            <option value="medium">中</option>
            <option value="high">高</option>
          </select>
          <button className="btn secondary" type="button" onClick={() => void loadArticles()} disabled={loading}>
            {loading ? "刷新中..." : "刷新"}
          </button>
          <button className="btn secondary" type="button" onClick={() => void handleSemanticSearch()} disabled={loading}>
            语义检索
          </button>
          <button className="btn" type="button" onClick={() => void handleImportDocs()} disabled={importing}>
            {importing ? "导入中..." : "导入 docs"}
          </button>
        </div>
        {error ? <div className="knowledge-error">{error}</div> : null}
        {message ? <div className="knowledge-message">{message}</div> : null}
      </section>

      <section className="panel knowledge-metrics">
        <div className="section-title">
          <h2>知识库运营指标</h2>
          <span>{metricsLoading ? "刷新中..." : "来源 /metrics"}</span>
        </div>
        <div className="knowledge-metrics-grid">
          <div className={`knowledge-metric-card ${searchHitRatio !== null && searchHitRatio < 0.7 ? "warn" : ""}`}>
            <div className="knowledge-metric-label">检索命中率</div>
            <div className="knowledge-metric-value">{formatRatio(searchHitRatio)}</div>
            <div className="knowledge-metric-desc">阈值建议 ≥ 70%</div>
          </div>
          <div className={`knowledge-metric-card ${askCitationRatio !== null && askCitationRatio < 0.95 ? "warn" : ""}`}>
            <div className="knowledge-metric-label">问答引用率</div>
            <div className="knowledge-metric-value">{formatRatio(askCitationRatio)}</div>
            <div className="knowledge-metric-desc">阈值建议 ≥ 95%</div>
          </div>
          <div className={`knowledge-metric-card ${reviewLatencyP95 !== null && reviewLatencyP95 > 800 ? "warn" : ""}`}>
            <div className="knowledge-metric-label">评审延迟 P95</div>
            <div className="knowledge-metric-value">{formatLatencyMs(reviewLatencyP95)}</div>
            <div className="knowledge-metric-desc">阈值建议 ≤ 800ms</div>
          </div>
        </div>
        <div className="knowledge-metrics-actions">
          <button className="btn secondary" type="button" onClick={() => void loadKBMetrics()} disabled={metricsLoading}>
            {metricsLoading ? "刷新中..." : "刷新指标"}
          </button>
        </div>
      </section>

      <section className="panel knowledge-pending">
        <div className="section-title">
          <h2>待审核 / 待复审队列</h2>
          <span>{pendingLoading ? "加载中..." : `${pendingReviews.length} 条`}</span>
        </div>
        {pendingReviews.length === 0 ? (
          <div className="empty-state">当前暂无待处理条目</div>
        ) : (
          <div className="knowledge-pending-list">
            {pendingReviews.map((item) => (
              <button
                className={`knowledge-pending-item ${item.id === selectedID ? "active" : ""}`}
                key={`pending-${item.id}`}
                type="button"
                onClick={() => setSelectedID(item.id)}
              >
                <div>
                  <strong>{item.title}</strong>
                  <div className="muted small">
                    {item.status === "draft" ? "待审核草稿" : item.needsReview ? "已到期待复审" : item.status}
                  </div>
                </div>
                <div className="knowledge-item-meta">
                  <span className="badge ghost">v{item.currentVersion}</span>
                  <span className="badge ghost">{item.severity}</span>
                </div>
              </button>
            ))}
          </div>
        )}
      </section>

      <div className="knowledge-layout">
        <section className="panel knowledge-list">
          <div className="section-title">
            <h2>条目列表</h2>
          </div>
          <div className="knowledge-list-body">
            {articles.length === 0 ? <div className="empty-state">暂无匹配条目</div> : null}
            {articles.map((item) => (
              <button
                key={item.id}
                type="button"
                className={`knowledge-item ${item.id === selectedID ? "active" : ""}`}
                onClick={() => setSelectedID(item.id)}
              >
                <div className="knowledge-item-main">
                  <strong>{item.title}</strong>
                  <span>{item.summary || "无摘要"}</span>
                </div>
                <div className="knowledge-item-meta">
                  <span className="badge ghost">{item.status}</span>
                  <span className="badge ghost">v{item.currentVersion}</span>
                  <span className="badge ghost">{item.severity}</span>
                </div>
              </button>
            ))}
          </div>
        </section>

        <section className="panel knowledge-editor">
          <div className="section-title">
            <h2>{selectedArticle ? `编辑条目 ${selectedArticle.id}` : "创建新条目"}</h2>
            {selectedDetail ? (
              <span>
                状态 {selectedDetail.status} · 当前版本 v{selectedDetail.currentVersion}
                {selectedDetail.needsReview ? " · 已到期待复审" : ""}
              </span>
            ) : null}
          </div>
          <div className="knowledge-form-grid">
            <div className="input">
              <label>标题</label>
              <input value={form.title} onChange={(e) => setForm((prev) => ({ ...prev, title: e.target.value }))} />
            </div>
            <div className="input">
              <label>摘要</label>
              <input value={form.summary} onChange={(e) => setForm((prev) => ({ ...prev, summary: e.target.value }))} />
            </div>
            <div className="input">
              <label>分类</label>
              <input value={form.category} onChange={(e) => setForm((prev) => ({ ...prev, category: e.target.value }))} />
            </div>
            <div className="input">
              <label>严重级别</label>
              <select
                className="select"
                value={form.severity}
                onChange={(e) => setForm((prev) => ({ ...prev, severity: e.target.value as Severity }))}
              >
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </div>
            <div className="input">
              <label>标签（逗号分隔）</label>
              <input value={form.tags} onChange={(e) => setForm((prev) => ({ ...prev, tags: e.target.value }))} />
            </div>
            <div className="input">
              <label>变更说明</label>
              <input
                value={form.changeNote}
                onChange={(e) => setForm((prev) => ({ ...prev, changeNote: e.target.value }))}
              />
            </div>
            <div className="input input-span">
              <div className="knowledge-editor-tabs">
                <button
                  className={`chip ${editorMode === "edit" ? "active" : ""}`}
                  type="button"
                  onClick={() => setEditorMode("edit")}
                >
                  编辑
                </button>
                <button
                  className={`chip ${editorMode === "preview" ? "active" : ""}`}
                  type="button"
                  onClick={() => setEditorMode("preview")}
                >
                  预览
                </button>
                <button
                  className={`chip ${editorMode === "diff" ? "active" : ""}`}
                  type="button"
                  onClick={() => setEditorMode("diff")}
                  disabled={!selectedID}
                >
                  Diff
                </button>
              </div>
              <label>正文（Markdown）</label>
              {editorMode === "edit" ? (
                <textarea
                  rows={14}
                  value={form.content}
                  onChange={(e) => setForm((prev) => ({ ...prev, content: e.target.value }))}
                />
              ) : editorMode === "preview" ? (
                <div className="knowledge-preview-box">{renderMarkdownPreview(form.content)}</div>
              ) : (
                <div className="knowledge-diff-box">
                  {diffLines.length === 0 ? (
                    <div className="empty-state">暂无差异</div>
                  ) : (
                    diffLines.map((line, index) => (
                      <div key={`diff-${index}`} className={`diff-line diff-${line.kind}`}>
                        <code>{line.text || " "}</code>
                      </div>
                    ))
                  )}
                </div>
              )}
              {selectedDetail?.versions && selectedDetail.versions.length > 0 ? (
                <div className="knowledge-version-line">
                  版本序列：{selectedDetail.versions.map((item) => `v${item.version}`).join(" -> ")}
                </div>
              ) : null}
            </div>
          </div>
          <div className="knowledge-actions">
            <button className="btn" type="button" onClick={() => void handleCreate()} disabled={saving}>
              新建
            </button>
            <button className="btn secondary" type="button" onClick={() => void handleSave()} disabled={saving || !selectedID}>
              保存
            </button>
            <button className="btn secondary" type="button" onClick={() => void handleAction("submit")} disabled={saving || !selectedID}>
              提交审核
            </button>
            <button className="btn secondary" type="button" onClick={() => void handleAction("approve")} disabled={saving || !selectedID}>
              发布
            </button>
            <button className="btn secondary" type="button" onClick={() => void handleAction("reject")} disabled={saving || !selectedID}>
              驳回
            </button>
            <button className="btn secondary" type="button" onClick={() => void handleAction("archive")} disabled={saving || !selectedID}>
              归档
            </button>
          </div>
          <div className="knowledge-rollback">
            <input
              className="search"
              placeholder="回滚目标版本号"
              value={rollbackVersion}
              onChange={(e) => setRollbackVersion(e.target.value)}
            />
            <button className="btn secondary" type="button" onClick={() => void handleRollback()} disabled={saving || !selectedID}>
              执行回滚
            </button>
          </div>
        </section>
      </div>

      <section className="panel knowledge-ask">
        <div className="section-title">
          <h2>知识问答</h2>
          <span>问答结果包含引用来源</span>
        </div>
        <div className="knowledge-toolbar">
          <input
            className="search"
            placeholder="例如：上传队列堆积如何排查？"
            value={askQuestion}
            onChange={(e) => setAskQuestion(e.target.value)}
          />
          <button className="btn" type="button" onClick={() => void handleAsk()} disabled={asking}>
            {asking ? "生成中..." : "提问"}
          </button>
        </div>
        {askResult ? (
          <div className="knowledge-answer">
            <div className="knowledge-answer-main">{askResult.answer}</div>
            <div className="knowledge-answer-meta">可信度 {Math.round((askResult.confidence ?? 0) * 100)}%</div>
            {askResult.meta?.degraded ? (
              <div className="knowledge-ask-meta degraded">
                <span className="badge ghost">degraded=true</span>
                <span className="badge ghost">errorClass={askResult.meta.errorClass || "unknown"}</span>
                <span className="badge ghost">回退依据={askResult.meta.fallbackReason || "fallback"}</span>
              </div>
            ) : (
              <div className="knowledge-ask-meta">
                <span className="badge ghost">degraded=false</span>
              </div>
            )}
            <div className="knowledge-citations">
              {(askResult.citations ?? []).map((item) => (
                <span className="badge ghost" key={`${item.articleId}-${item.version}`}>
                  {item.title} (v{item.version})
                </span>
              ))}
            </div>
          </div>
        ) : (
          <div className="empty-state">输入问题后可生成基于知识库的引用式回答</div>
        )}
      </section>
    </div>
  );
}


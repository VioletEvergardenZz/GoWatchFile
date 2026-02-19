/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于控制台 API 封装 将网络请求统一收敛便于复用和排障 */

import type {
  AiLogSummaryResponse,
  ControlAuditLogsResponse,
  ControlAgentsResponse,
  ControlTaskFailureReasonsResponse,
  ControlTaskEventsResponse,
  ControlTaskResponse,
  ControlTasksResponse,
  DashboardPayload,
  KnowledgeArticleResponse,
  KnowledgeAskResponse,
  KnowledgeImportResponse,
  KnowledgeListResponse,
  KnowledgePendingReviewsResponse,
  KnowledgeRecommendationsResponse,
  KnowledgeSearchResponse,
} from "../types";

export const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
export const USE_MOCK = ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";
const API_TOKEN_STORAGE_KEY = "gwf-api-token";
let runtimeApiToken = "";

// token 读取优先级为 session -> local
// 这样默认“仅当前会话有效”，显式勾选记住后才落到 localStorage
const readTokenFromStorage = () => {
  if (typeof window === "undefined") return "";
  const fromSession = window.sessionStorage.getItem(API_TOKEN_STORAGE_KEY);
  if (fromSession) return fromSession.trim();
  const fromLocal = window.localStorage.getItem(API_TOKEN_STORAGE_KEY);
  if (fromLocal) return fromLocal.trim();
  return "";
};

export const getApiToken = () => {
  if (runtimeApiToken) return runtimeApiToken;
  runtimeApiToken = readTokenFromStorage();
  return runtimeApiToken;
};

export const isApiTokenRemembered = () => {
  if (typeof window === "undefined") return false;
  return !!window.localStorage.getItem(API_TOKEN_STORAGE_KEY);
};

export const setApiToken = (token: string, remember = false) => {
  const normalized = token.trim();
  runtimeApiToken = normalized;
  if (typeof window === "undefined") return;

  if (!normalized) {
    window.sessionStorage.removeItem(API_TOKEN_STORAGE_KEY);
    window.localStorage.removeItem(API_TOKEN_STORAGE_KEY);
    return;
  }

  if (remember) {
    window.localStorage.setItem(API_TOKEN_STORAGE_KEY, normalized);
    window.sessionStorage.removeItem(API_TOKEN_STORAGE_KEY);
    return;
  }
  window.sessionStorage.setItem(API_TOKEN_STORAGE_KEY, normalized);
  window.localStorage.removeItem(API_TOKEN_STORAGE_KEY);
};

export const clearApiToken = () => {
  runtimeApiToken = "";
  if (typeof window === "undefined") return;
  window.sessionStorage.removeItem(API_TOKEN_STORAGE_KEY);
  window.localStorage.removeItem(API_TOKEN_STORAGE_KEY);
};

export type LogMode = "tail" | "search";

export type FileLogResponse = {
  lines?: string[];
  mode?: LogMode;
  query?: string;
  matched?: number;
  truncated?: boolean;
};

type ConfigResponse = {
  ok?: boolean;
  config?: {
    watchDir: string;
    fileExt: string;
    concurrency?: string;
    silence?: string;
    uploadRetryDelays?: string;
    uploadRetryEnabled?: boolean;
    systemResourceEnabled?: boolean;
  };
};

export const buildApiHeaders = (contentType = false): HeadersInit => {
  const headers: Record<string, string> = {};
  if (contentType) {
    headers["Content-Type"] = "application/json";
  }
  const token = getApiToken();
  if (token) {
    headers["X-API-Token"] = token;
  }
  return headers;
};

// ensureOk 统一把非 2xx 响应转换为可读错误
// 页面层只处理业务状态 不重复拼装状态码和错误文本
const readErrorDetail = async (res: Response) => {
  const text = await res.text();
  if (!text) return "";
  try {
    const payload = JSON.parse(text) as { error?: string };
    if (payload?.error) return payload.error.trim();
  } catch (error) {
    void error;
  }
  return text.trim();
};

const formatErrorMessage = (action: string, res: Response, detail: string) => {
  const statusText = res.statusText ? ` ${res.statusText}` : "";
  const base = `${action}失败，状态码 ${res.status}${statusText}`;
  if (!detail) {
    return `${base}，请确认后端服务可用且接口地址配置正确`;
  }
  return `${base}，后端提示：${detail}`;
};

const ensureOk = async (res: Response, action: string) => {
  if (res.ok) return;
  const detail = await readErrorDetail(res);
  throw new Error(formatErrorMessage(action, res, detail));
};

// 下面是业务 API 封装
// 约定每个函数只做一件事 请求 + 基本错误归一化 + 类型化返回
export const fetchDashboard = async (): Promise<Partial<DashboardPayload>> => {
  const res = await fetch(`${API_BASE}/api/dashboard`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "仪表盘数据加载");
  return (await res.json()) as Partial<DashboardPayload>;
};

export const fetchDashboardLite = async (): Promise<Partial<DashboardPayload>> => {
  const res = await fetch(`${API_BASE}/api/dashboard?mode=light`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "实时数据刷新");
  return (await res.json()) as Partial<DashboardPayload>;
};

export const postAutoUpload = async (path: string, enabled: boolean) => {
  const res = await fetch(`${API_BASE}/api/auto-upload`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify({ path, enabled }),
  });
  await ensureOk(res, "自动上传开关更新");
};

export const postManualUpload = async (path: string) => {
  const res = await fetch(`${API_BASE}/api/manual-upload`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify({ path }),
  });
  await ensureOk(res, "手动上传触发");
};

export const postConfig = async (payload: {
  watchDir: string;
  fileExt: string;
  uploadWorkers: number;
  uploadQueueSize: number;
  uploadRetryDelays: string;
  uploadRetryEnabled: boolean;
  silence: string;
  systemResourceEnabled: boolean;
}): Promise<ConfigResponse> => {
  const res = await fetch(`${API_BASE}/api/config`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "配置保存");
  return (await res.json()) as ConfigResponse;
};

export const postSystemResourceEnabled = async (enabled: boolean): Promise<ConfigResponse> => {
  const res = await fetch(`${API_BASE}/api/config`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify({ systemResourceEnabled: enabled }),
  });
  await ensureOk(res, "系统资源控制台开关更新");
  return (await res.json()) as ConfigResponse;
};

export const postFileLog = async (payload: { path: string; query?: string }): Promise<FileLogResponse> => {
  const res = await fetch(`${API_BASE}/api/file-log`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "文件内容读取");
  return (await res.json()) as FileLogResponse;
};

export type AiLogSummaryRequest = {
  path: string;
  mode?: LogMode;
  query?: string;
  limit?: number;
  caseSensitive?: boolean;
};

export const postAiLogSummary = async (payload: AiLogSummaryRequest): Promise<AiLogSummaryResponse> => {
  const res = await fetch(`${API_BASE}/api/ai/log-summary`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "AI日志分析");
  return (await res.json()) as AiLogSummaryResponse;
};

export type KnowledgeArticlePayload = {
  title: string;
  summary: string;
  category: string;
  severity: "low" | "medium" | "high";
  content: string;
  tags: string[];
  changeNote?: string;
  createdBy?: string;
  updatedBy?: string;
  sourceType?: string;
  sourceRef?: string;
  refTitle?: string;
};

export const fetchKBArticles = async (params?: {
  q?: string;
  status?: string;
  severity?: string;
  tag?: string;
  page?: number;
  pageSize?: number;
  includeArchived?: boolean;
}): Promise<KnowledgeListResponse> => {
  const query = new URLSearchParams();
  if (params?.q) query.set("q", params.q);
  if (params?.status) query.set("status", params.status);
  if (params?.severity) query.set("severity", params.severity);
  if (params?.tag) query.set("tag", params.tag);
  if (params?.page) query.set("page", String(params.page));
  if (params?.pageSize) query.set("pageSize", String(params.pageSize));
  if (params?.includeArchived) query.set("includeArchived", "true");
  const suffix = query.toString();
  const res = await fetch(`${API_BASE}/api/kb/articles${suffix ? `?${suffix}` : ""}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "知识条目加载");
  return (await res.json()) as KnowledgeListResponse;
};

export const fetchKBArticle = async (id: string): Promise<KnowledgeArticleResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/articles/${encodeURIComponent(id)}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "知识详情加载");
  return (await res.json()) as KnowledgeArticleResponse;
};

export const postKBArticle = async (payload: KnowledgeArticlePayload): Promise<KnowledgeArticleResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/articles`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "创建知识条目");
  return (await res.json()) as KnowledgeArticleResponse;
};

export const putKBArticle = async (id: string, payload: KnowledgeArticlePayload): Promise<KnowledgeArticleResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/articles/${encodeURIComponent(id)}`, {
    method: "PUT",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "更新知识条目");
  return (await res.json()) as KnowledgeArticleResponse;
};

export const postKBArticleAction = async (
  id: string,
  action: "submit" | "approve" | "reject" | "archive",
  payload?: { operator?: string; comment?: string }
): Promise<KnowledgeArticleResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/articles/${encodeURIComponent(id)}/${action}`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload ?? {}),
  });
  await ensureOk(res, "知识状态更新");
  return (await res.json()) as KnowledgeArticleResponse;
};

export const postKBRollback = async (
  id: string,
  payload: { targetVersion: number; operator?: string; comment?: string }
): Promise<KnowledgeArticleResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/articles/${encodeURIComponent(id)}/rollback`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "知识版本回滚");
  return (await res.json()) as KnowledgeArticleResponse;
};

export const postKBSearch = async (payload: {
  query: string;
  limit?: number;
  includeArchived?: boolean;
}): Promise<KnowledgeSearchResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/search`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "知识检索");
  return (await res.json()) as KnowledgeSearchResponse;
};

export const postKBAsk = async (payload: { question: string; limit?: number }): Promise<KnowledgeAskResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/ask`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "知识问答");
  return (await res.json()) as KnowledgeAskResponse;
};

export const postKBImportDocs = async (payload?: { path?: string; operator?: string }): Promise<KnowledgeImportResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/import/docs`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify(payload ?? {}),
  });
  await ensureOk(res, "文档导入");
  return (await res.json()) as KnowledgeImportResponse;
};

export const fetchKBPendingReviews = async (limit = 20): Promise<KnowledgePendingReviewsResponse> => {
  const res = await fetch(`${API_BASE}/api/kb/reviews/pending?limit=${encodeURIComponent(String(limit))}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "待复审队列加载");
  return (await res.json()) as KnowledgePendingReviewsResponse;
};

export const fetchKBRecommendations = async (params: {
  query?: string;
  rule?: string;
  message?: string;
  alertId?: string;
  limit?: number;
}): Promise<KnowledgeRecommendationsResponse> => {
  const query = new URLSearchParams();
  if (params.query) query.set("query", params.query);
  if (params.rule) query.set("rule", params.rule);
  if (params.message) query.set("message", params.message);
  if (params.alertId) query.set("alertId", params.alertId);
  if (params.limit) query.set("limit", String(params.limit));
  const suffix = query.toString();
  const res = await fetch(`${API_BASE}/api/kb/recommendations${suffix ? `?${suffix}` : ""}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "知识推荐加载");
  return (await res.json()) as KnowledgeRecommendationsResponse;
};

export type KnowledgeMetricsSnapshot = {
  searchHitRatio: number | null;
  askCitationRatio: number | null;
  reviewLatencyP95Ms: number | null;
};

const parseMetricValue = (text: string, metricName: string): number | null => {
  const pattern = new RegExp(`^${metricName}(?:\\{[^\\n]*\\})?\\s+([0-9.eE+-]+)$`, "m");
  const match = text.match(pattern);
  if (!match) return null;
  const value = Number.parseFloat(match[1]);
  if (!Number.isFinite(value)) return null;
  return value;
};

const parseReviewLatencyP95 = (text: string): number | null => {
  const total = parseMetricValue(text, "gwf_kb_review_latency_ms_count");
  if (total === null || total <= 0) {
    return null;
  }
  const bucketPattern = /^gwf_kb_review_latency_ms_bucket\{[^}]*le="([^"]+)"[^}]*\}\s+([0-9.eE+-]+)$/gm;
  const buckets: Array<{ le: number; count: number }> = [];
  let match: RegExpExecArray | null;
  while ((match = bucketPattern.exec(text)) !== null) {
    if (!match[1] || match[1] === "+Inf") {
      continue;
    }
    const le = Number.parseFloat(match[1]);
    const count = Number.parseFloat(match[2]);
    if (!Number.isFinite(le) || !Number.isFinite(count)) {
      continue;
    }
    buckets.push({ le, count });
  }
  if (buckets.length === 0) {
    return null;
  }
  buckets.sort((a, b) => a.le - b.le);
  const target = total * 0.95;
  for (const bucket of buckets) {
    if (bucket.count >= target) {
      return bucket.le;
    }
  }
  return buckets[buckets.length - 1]?.le ?? null;
};

export const fetchKBMetrics = async (): Promise<KnowledgeMetricsSnapshot> => {
  const res = await fetch(`${API_BASE}/metrics`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "知识库指标加载");
  const text = await res.text();
  return {
    searchHitRatio: parseMetricValue(text, "gwf_kb_search_hit_ratio"),
    askCitationRatio: parseMetricValue(text, "gwf_kb_ask_citation_ratio"),
    reviewLatencyP95Ms: parseReviewLatencyP95(text),
  };
};

export const fetchControlAgents = async (params?: { status?: string; group?: string }): Promise<ControlAgentsResponse> => {
  const query = new URLSearchParams();
  if (params?.status) query.set("status", params.status);
  if (params?.group) query.set("group", params.group);
  const suffix = query.toString();
  const res = await fetch(`${API_BASE}/api/control/agents${suffix ? `?${suffix}` : ""}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "控制面Agent加载");
  return (await res.json()) as ControlAgentsResponse;
};

export const fetchControlTasks = async (params?: { status?: string; type?: string; limit?: number }): Promise<ControlTasksResponse> => {
  const query = new URLSearchParams();
  if (params?.status) query.set("status", params.status);
  if (params?.type) query.set("type", params.type);
  if (params?.limit) query.set("limit", String(params.limit));
  const suffix = query.toString();
  const res = await fetch(`${API_BASE}/api/control/tasks${suffix ? `?${suffix}` : ""}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "控制面任务加载");
  return (await res.json()) as ControlTasksResponse;
};

export const fetchControlTaskEvents = async (taskId: string, limit = 200): Promise<ControlTaskEventsResponse> => {
  const id = encodeURIComponent(taskId);
  const res = await fetch(`${API_BASE}/api/control/tasks/${id}/events?limit=${encodeURIComponent(String(limit))}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "控制面任务事件加载");
  return (await res.json()) as ControlTaskEventsResponse;
};

export const fetchControlTaskFailureReasons = async (params?: {
  status?: string;
  type?: string;
  limit?: number;
}): Promise<ControlTaskFailureReasonsResponse> => {
  const query = new URLSearchParams();
  if (params?.status) query.set("status", params.status);
  if (params?.type) query.set("type", params.type);
  if (params?.limit) query.set("limit", String(params.limit));
  const suffix = query.toString();
  const res = await fetch(`${API_BASE}/api/control/tasks/failure-reasons${suffix ? `?${suffix}` : ""}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "控制面任务失败原因分布加载");
  return (await res.json()) as ControlTaskFailureReasonsResponse;
};
export const postControlTaskCancel = async (taskId: string): Promise<ControlTaskResponse> => {
  const id = encodeURIComponent(taskId);
  const res = await fetch(`${API_BASE}/api/control/tasks/${id}/cancel`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify({}),
  });
  await ensureOk(res, "控制面任务取消");
  return (await res.json()) as ControlTaskResponse;
};

export const postControlTaskRetry = async (taskId: string): Promise<ControlTaskResponse> => {
  const id = encodeURIComponent(taskId);
  const res = await fetch(`${API_BASE}/api/control/tasks/${id}/retry`, {
    method: "POST",
    headers: buildApiHeaders(true),
    body: JSON.stringify({}),
  });
  await ensureOk(res, "控制面任务重试");
  return (await res.json()) as ControlTaskResponse;
};

export const fetchControlAuditLogs = async (params?: {
  resourceType?: string;
  resourceId?: string;
  operator?: string;
  action?: string;
  from?: string;
  to?: string;
  limit?: number;
}): Promise<ControlAuditLogsResponse> => {
  const query = new URLSearchParams();
  if (params?.resourceType) query.set("resourceType", params.resourceType);
  if (params?.resourceId) query.set("resourceId", params.resourceId);
  if (params?.operator) query.set("operator", params.operator);
  if (params?.action) query.set("action", params.action);
  if (params?.from) query.set("from", params.from);
  if (params?.to) query.set("to", params.to);
  if (params?.limit) query.set("limit", String(params.limit));
  const suffix = query.toString();
  const res = await fetch(`${API_BASE}/api/control/audit${suffix ? `?${suffix}` : ""}`, {
    headers: buildApiHeaders(),
  });
  await ensureOk(res, "控制面审计日志加载");
  return (await res.json()) as ControlAuditLogsResponse;
};



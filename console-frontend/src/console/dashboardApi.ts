import type { AiLogSummaryResponse, DashboardPayload } from "../types";

export const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
export const USE_MOCK = ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";
const API_TOKEN_STORAGE_KEY = "gwf-api-token";
let runtimeApiToken = "";

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

import type { DashboardPayload } from "../types";

export const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
export const USE_MOCK = ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";

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
  config?: { watchDir: string; fileExt: string; concurrency?: string; silence?: string; systemResourceEnabled?: boolean };
};

const readErrorDetail = async (res: Response) => {
  const text = await res.text();
  if (!text) return "";
  try {
    const payload = JSON.parse(text) as { error?: string };
    if (payload?.error) return payload.error.trim();
  } catch {}
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
  const res = await fetch(`${API_BASE}/api/dashboard`);
  await ensureOk(res, "仪表盘数据加载");
  return (await res.json()) as Partial<DashboardPayload>;
};

export const fetchDashboardLite = async (): Promise<Partial<DashboardPayload>> => {
  const res = await fetch(`${API_BASE}/api/dashboard?mode=light`);
  await ensureOk(res, "实时数据刷新");
  return (await res.json()) as Partial<DashboardPayload>;
};

export const postAutoUpload = async (path: string, enabled: boolean) => {
  const res = await fetch(`${API_BASE}/api/auto-upload`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path, enabled }),
  });
  await ensureOk(res, "自动上传开关更新");
};

export const postManualUpload = async (path: string) => {
  const res = await fetch(`${API_BASE}/api/manual-upload`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
  });
  await ensureOk(res, "手动上传触发");
};

export const postConfig = async (payload: {
  watchDir: string;
  fileExt: string;
  uploadWorkers: number;
  uploadQueueSize: number;
  silence: string;
  systemResourceEnabled: boolean;
}): Promise<ConfigResponse> => {
  const res = await fetch(`${API_BASE}/api/config`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "配置保存");
  return (await res.json()) as ConfigResponse;
};

export const postFileLog = async (payload: { path: string; query?: string }): Promise<FileLogResponse> => {
  const res = await fetch(`${API_BASE}/api/file-log`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  await ensureOk(res, "文件内容读取");
  return (await res.json()) as FileLogResponse;
};

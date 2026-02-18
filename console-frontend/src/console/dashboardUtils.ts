/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于控制台数据处理工具 统一格式化和树结构构建逻辑 */

import type { ConfigSnapshot, FileNode, UploadRecord } from "../types";

const hasDatePrefix = (value: string) => /\d{4}-\d{2}-\d{2}/.test(value);

const localDatePrefix = () => {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
};

export const fmt = (t: string) => {
  if (!t || t === "--") return t || "--";
  if (hasDatePrefix(t)) return t;
  return `${localDatePrefix()} ${t}`;
};

export const resolveRecordTimestamp = (value: string) => {
  if (!value || value === "--") return 0;
  const normalized = hasDatePrefix(value) ? value.replace(" ", "T") : `${localDatePrefix()}T${value}`;
  const parsed = Date.parse(normalized);
  return Number.isNaN(parsed) ? 0 : parsed;
};

const WATCH_DIR_CONFIG_SPLIT_RE = /[,\n\r;，；]+/;

export const splitWatchDirs = (raw: string) =>
  raw
    .split(WATCH_DIR_CONFIG_SPLIT_RE)
    .map((dir) => dir.trim())
    .filter(Boolean);

const isWatchDirInputSeparator = (value: string) =>
  value === "," ||
  value === ";" ||
  value === "，" ||
  value === "；" ||
  value === "\n" ||
  value === "\r" ||
  value === "\t" ||
  value === " ";

export const splitWatchDirsInput = (raw: string) => {
  const out: string[] = [];
  let buffer = "";
  let quote: "'" | '"' | null = null;
  let escaped = false;

  const push = () => {
    const trimmed = buffer.trim();
    if (trimmed) out.push(trimmed);
    buffer = "";
  };

  for (let i = 0; i < raw.length; i += 1) {
    const ch = raw[i];
    if (escaped) {
      buffer += ch;
      escaped = false;
      continue;
    }
    if (quote) {
      if (ch === "\\") {
        escaped = true;
        continue;
      }
      if (ch === quote) {
        quote = null;
        continue;
      }
      buffer += ch;
      continue;
    }
    if (ch === "'" || ch === "\"") {
      quote = ch;
      continue;
    }
    if (isWatchDirInputSeparator(ch)) {
      push();
      continue;
    }
    buffer += ch;
  }
  if (escaped) {
    buffer += "\\";
  }
  push();
  return out;
};

export const normalizeWatchDirInput = (raw: string) => splitWatchDirsInput(raw).join(",");

const splitFileExtList = (raw: string) =>
  raw
    .split(/[,\s;]+/)
    .map((item) => item.trim())
    .filter(Boolean);

const resolveUploadStatusQuery = (raw: string) => {
  const cleaned = raw.trim().toLowerCase();
  if (!cleaned) return "";
  if (cleaned.includes("成功") || cleaned.includes("success") || cleaned.includes("uploaded") || cleaned.includes("ok")) {
    return "success";
  }
  if (cleaned.includes("失败") || cleaned.includes("failed") || cleaned.includes("error") || cleaned.includes("fail")) {
    return "failed";
  }
  if (cleaned.includes("排队") || cleaned.includes("队列") || cleaned.includes("等待") || cleaned.includes("pending") || cleaned.includes("queued")) {
    return "pending";
  }
  return "";
};

export const matchUploadSearch = (record: UploadRecord, raw: string) => {
  const trimmed = raw.trim();
  if (!trimmed) return true;
  const status = resolveUploadStatusQuery(trimmed);
  if (status) {
    return record.result === status;
  }
  const haystack = `${record.file} ${record.target ?? ""} ${record.note ?? ""}`.toLowerCase();
  return haystack.includes(trimmed.toLowerCase());
};

export const formatTreeFilterBadge = (raw: string) => {
  const trimmed = raw.trim();
  if (!trimmed) return "--";
  if (trimmed.includes("全量")) return "全量目录";
  const cleaned = trimmed.replace(/^过滤\\s*/i, "");
  const exts = splitFileExtList(cleaned);
  if (!exts.length) return trimmed;
  return `过滤 ${exts.join(", ")}`;
};

export const normalizeConfigSnapshot = (value?: Partial<ConfigSnapshot>): ConfigSnapshot => {
  const base = value ?? {};
  return {
    watchDir: base.watchDir ?? "",
    fileExt: base.fileExt ?? "",
    silence: base.silence ?? "",
    concurrency: base.concurrency ?? "",
    uploadRetryDelays: base.uploadRetryDelays ?? "",
    uploadRetryEnabled: base.uploadRetryEnabled ?? true,
    systemResourceEnabled: base.systemResourceEnabled ?? false,
  };
};

const retryDelayTokenPattern = /^\d+(ms|s|m|h)$/i;

export const validateRetryDelays = (raw: string): string => {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "";
  }
  const parts = trimmed.split(/[\s,;]+/).filter(Boolean);
  if (parts.length === 0) {
    return "";
  }
  for (const part of parts) {
    if (!retryDelayTokenPattern.test(part)) {
      return "格式不合法，示例：500ms,1s,2s（只支持 ms/s/m/h）";
    }
  }
  return "";
};

export const findFirstFile = (nodes: FileNode[]): FileNode | undefined => {
  for (const node of nodes) {
    if (node.type === "file") return node;
    if (node.children) {
      const child = findFirstFile(node.children);
      if (child) return child;
    }
  }
  return undefined;
};

export const findNode = (nodes: FileNode[], path: string): FileNode | undefined => {
  for (const node of nodes) {
    if (node.path === path) return node;
    if (node.children) {
      const found = findNode(node.children, path);
      if (found) return found;
    }
  }
  return undefined;
};

export const collectDirPaths = (nodes: FileNode[]): string[] => {
  const paths: string[] = [];
  const walk = (items: FileNode[]) => {
    items.forEach((node) => {
      if (node.type !== "dir") return;
      paths.push(node.path);
      if (node.children) walk(node.children);
    });
  };
  walk(nodes);
  return paths;
};

const propagateAuto = (children: FileNode[] | undefined, value: boolean): FileNode[] | undefined => {
  if (!children) return children;
  return children.map((child) => ({
    ...child,
    autoUpload: value,
    children: propagateAuto(child.children, value),
  }));
};

export const updateAutoUpload = (nodes: FileNode[], path: string, value: boolean): FileNode[] => {
  return nodes.map((node) => {
    if (node.path === path) {
      return {
        ...node,
        autoUpload: value,
        children: propagateAuto(node.children, value),
      };
    }
    if (node.children) {
      return {
        ...node,
        children: updateAutoUpload(node.children, path, value),
      };
    }
    return node;
  });
};


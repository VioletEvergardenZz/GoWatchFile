/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于前端应用入口组件 负责视图切换与初始化状态控制 */

import { useEffect, useState } from "react";
import "./App.css";
import { OriginalConsole } from "./OriginalConsole";

const VIEW_STORAGE_KEY = "gwf-console-view";

type ViewType = "console" | "alert" | "system" | "knowledge" | "control";

const resolveInitialView = (): ViewType => {
  if (typeof window === "undefined") return "console";
  const stored = window.localStorage.getItem(VIEW_STORAGE_KEY);
  if (stored === "alert") return "alert";
  if (stored === "system") return "system";
  if (stored === "knowledge") return "knowledge";
  if (stored === "control") return "control";
  return "console";
};

function App() {
  const [view, setView] = useState<ViewType>(() => resolveInitialView());

  useEffect(() => {
    if (typeof window === "undefined") return;
    window.localStorage.setItem(VIEW_STORAGE_KEY, view);
  }, [view]);

  return <OriginalConsole view={view} onViewChange={setView} />;
}

export default App;


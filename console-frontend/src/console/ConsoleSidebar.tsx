/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于主控制台侧栏组件 负责视图切换和导航入口管理 */

type ConsoleView = "console" | "alert" | "system" | "knowledge" | "control";

type ConsoleSidebarProps = {
  view: ConsoleView;
  activeSection: string;
  sectionIds: string[];
  systemSectionIds: string[];
  onViewChange: (view: ConsoleView) => void;
};

const resolveConsoleMeta = (id: string) => {
  switch (id) {
    case "overview":
      return { title: "总览", desc: "心跳 · 策略摘要", badge: "状态" };
    case "config":
      return { title: "上传与路由配置", desc: "本机配置 · 路由", badge: "配置" };
    case "directory":
      return { title: "目录浏览", desc: "单目录展开 / 收起", badge: "目录树" };
    case "files":
      return { title: "文件列表", desc: "基础信息", badge: "列表" };
    case "tail":
      return { title: "文件内容", desc: "文件内容", badge: "内容" };
    case "failures":
      return { title: "上传记录", desc: "最近上传", badge: "记录" };
    default:
      return { title: "监控", desc: "吞吐 / 队列", badge: "图表" };
  }
};

const resolveSystemMeta = (id: string) => {
  switch (id) {
    case "system-overview":
      return { title: "系统概览", desc: "主机 / 负载 / 连接", badge: "概览" };
    case "system-resources":
      return { title: "资源总览", desc: "CPU / 内存 / 磁盘", badge: "资源" };
    case "system-volumes":
      return { title: "磁盘分区", desc: "容量 / 使用率", badge: "分区" };
    case "system-processes":
      return { title: "进程列表", desc: "筛选 / 排序", badge: "进程" };
    default:
      return { title: "进程详情", desc: "指标 / 处置", badge: "详情" };
  }
};

export function ConsoleSidebar({ view, activeSection, sectionIds, systemSectionIds, onViewChange }: ConsoleSidebarProps) {
  return (
    <aside className="sidebar">
      <div className="nav-brand">
        <div className="brand-logo brand-logo-small">
          <div className="brand-logo-mark">GWF</div>
          <div className="brand-logo-sub">Go Watch File</div>
        </div>
      </div>
      <div className="view-switch" role="tablist" aria-label="控制台切换">
        <button
          className={`view-tab ${view === "console" ? "active" : ""}`}
          type="button"
          role="tab"
          aria-selected={view === "console"}
          onClick={() => onViewChange("console")}
        >
          文件监控控制台
        </button>
        <button
          className={`view-tab ${view === "alert" ? "active" : ""}`}
          type="button"
          role="tab"
          aria-selected={view === "alert"}
          onClick={() => onViewChange("alert")}
        >
          告警控制台
        </button>
        <button
          className={`view-tab ${view === "system" ? "active" : ""}`}
          type="button"
          role="tab"
          aria-selected={view === "system"}
          onClick={() => onViewChange("system")}
        >
          系统资源控制台
        </button>
        <button
          className={`view-tab ${view === "control" ? "active" : ""}`}
          type="button"
          role="tab"
          aria-selected={view === "control"}
          onClick={() => onViewChange("control")}
        >
          控制面
        </button>
        <button
          className={`view-tab ${view === "knowledge" ? "active" : ""}`}
          type="button"
          role="tab"
          aria-selected={view === "knowledge"}
          onClick={() => onViewChange("knowledge")}
        >
          运维知识库
        </button>
      </div>
      {view === "console" ? (
        <nav className="nav-list">
          {sectionIds.map((id) => {
            const { title, desc, badge } = resolveConsoleMeta(id);
            return (
              <a key={id} className={`nav-item ${activeSection === id ? "active" : ""}`} href={`#${id}`}>
                <div className="nav-label">
                  <span className={`nav-dot ${activeSection === id ? "live" : ""}`} />
                  <div>
                    <div className="nav-label-title">{title}</div>
                    <small>{desc}</small>
                  </div>
                </div>
                <span className="badge ghost">{badge}</span>
              </a>
            );
          })}
        </nav>
      ) : view === "system" ? (
        <nav className="nav-list">
          {systemSectionIds.map((id) => {
            const { title, desc, badge } = resolveSystemMeta(id);
            return (
              <a key={id} className={`nav-item ${activeSection === id ? "active" : ""}`} href={`#${id}`}>
                <div className="nav-label">
                  <span className={`nav-dot ${activeSection === id ? "live" : ""}`} />
                  <div>
                    <div className="nav-label-title">{title}</div>
                    <small>{desc}</small>
                  </div>
                </div>
                <span className="badge ghost">{badge}</span>
              </a>
            );
          })}
        </nav>
      ) : null}
    </aside>
  );
}


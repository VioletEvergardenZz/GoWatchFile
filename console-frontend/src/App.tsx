import { useEffect, useMemo, useState } from "react";
import { Line } from "react-chartjs-2";
import type { ChartOptions } from "chart.js";
import { CategoryScale, Chart as ChartJS, Filler, Legend, LineElement, LinearScale, PointElement, Tooltip } from "chart.js";
import "./App.css";
import {
  chartPoints,
  configSnapshot,
  directoryTree as treeSeed,
  failures,
  files as fileSeed,
  heroCopy,
  heroHighlights,
  metricCards,
  monitorNotes,
  routes,
  tailLines,
  timelineEvents,
} from "./mockData";
import type { FileFilter, FileItem, FileNode } from "./types";

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Filler, Legend);

const SECTION_IDS = ["overview", "directory", "files", "tail", "failures", "monitor"];

const fmt = (t: string) => `${new Date().toISOString().split("T")[0]} ${t}`;

const findFirstFile = (nodes: FileNode[]): FileNode | undefined => {
  for (const node of nodes) {
    if (node.type === "file") return node;
    if (node.children) {
      const child = findFirstFile(node.children);
      if (child) return child;
    }
  }
  return undefined;
};

const findNode = (nodes: FileNode[], path: string): FileNode | undefined => {
  for (const node of nodes) {
    if (node.path === path) return node;
    if (node.children) {
      const found = findNode(node.children, path);
      if (found) return found;
    }
  }
  return undefined;
};

const propagateAuto = (children: FileNode[] | undefined, value: boolean): FileNode[] | undefined => {
  if (!children) return children;
  return children.map((child) => ({
    ...child,
    autoUpload: value,
    children: propagateAuto(child.children, value),
  }));
};

const updateAutoUpload = (nodes: FileNode[], path: string, value: boolean): FileNode[] => {
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

function App() {
  const [tree, setTree] = useState<FileNode[]>(treeSeed);
  const [files, setFiles] = useState<FileItem[]>(fileSeed);
  const [currentRoot, setCurrentRoot] = useState<string>(treeSeed[0]?.path ?? "");
  const [activePath, setActivePath] = useState<string>(findFirstFile(treeSeed)?.path ?? "");
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const [fileFilter, setFileFilter] = useState<FileFilter>("all");
  const [searchTerm, setSearchTerm] = useState("");
  const [manualUploadMap, setManualUploadMap] = useState<Record<string, string>>({});
  const [activeSection, setActiveSection] = useState<string>(SECTION_IDS[0]);

  const rootNodes = useMemo(() => {
    const filtered = tree.filter((node) => !currentRoot || node.path === currentRoot);
    return filtered.length ? filtered : tree;
  }, [tree, currentRoot]);

  const activeNode = useMemo(() => {
    if (!activePath) return undefined;
    return findNode(rootNodes, activePath);
  }, [activePath, rootNodes]);

  useEffect(() => {
    const hasActiveUnderRoot = activePath && activePath.startsWith(currentRoot);
    if (hasActiveUnderRoot) return;
    const next = findFirstFile(rootNodes);
    setActivePath(next?.path ?? "");
  }, [currentRoot, rootNodes, activePath]);

  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((entry) => entry.isIntersecting)
          .sort((a, b) => b.intersectionRatio - a.intersectionRatio);
        if (visible[0]?.target?.id) {
          setActiveSection(visible[0].target.id);
        }
      },
      { threshold: [0.25, 0.5], rootMargin: "-30% 0px -30% 0px" }
    );

    const targets = SECTION_IDS.map((id) => document.getElementById(id)).filter(Boolean) as Element[];
    targets.forEach((el) => observer.observe(el));
    return () => observer.disconnect();
  }, []);

  const handleAutoToggle = (path: string, value: boolean) => {
    const node = findNode(tree, path);
    const isDir = node?.type === "dir";
    setTree((prev) => updateAutoUpload(prev, path, value));
    setFiles((prev) =>
      prev.map((f) => {
        if (f.path === path || (isDir && f.path.startsWith(`${path}/`))) {
          return { ...f, autoUpload: value };
        }
        return f;
      })
    );
  };

  const handleCollapseToggle = (path: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  };

  const handleCollapseAll = (collapse: boolean) => {
    if (!collapse) {
      setCollapsed(new Set());
      return;
    }
    const dirPaths: string[] = [];
    const walk = (nodes: FileNode[]) => {
      nodes.forEach((n) => {
        if (n.type === "dir") dirPaths.push(n.path);
        if (n.children) walk(n.children);
      });
    };
    walk(rootNodes);
    setCollapsed(new Set(dirPaths));
  };

  const handleManualUpload = () => {
    if (!activePath) return;
    const now = new Date().toTimeString().slice(0, 8);
    setManualUploadMap((prev) => ({ ...prev, [activePath]: now }));
  };

  const filteredFiles = useMemo(() => {
    return files
      .filter((f) => (currentRoot ? f.path.startsWith(currentRoot) : true))
      .filter((f) =>
        searchTerm.trim()
          ? f.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
            f.path.toLowerCase().includes(searchTerm.toLowerCase())
          : true
      )
      .filter((f) => {
        switch (fileFilter) {
          case "auto":
            return f.autoUpload;
          case "manual":
            return !f.autoUpload;
          case "approval":
            return !!f.requiresApproval;
          case "failed":
            return f.status === "failed";
          default:
            return true;
        }
      });
  }, [files, currentRoot, fileFilter, searchTerm]);

  const chartData = useMemo(
    () => ({
      labels: chartPoints.map((p) => p.label),
      datasets: [
        {
          label: "Uploads",
          data: chartPoints.map((p) => p.uploads),
          borderColor: "#22d3ee",
          backgroundColor: "rgba(34,211,238,0.12)",
          fill: true,
          tension: 0.35,
          pointRadius: 0,
        },
        {
          label: "Failures",
          data: chartPoints.map((p) => p.failures),
          borderColor: "#f43f5e",
          backgroundColor: "rgba(244,63,94,0.10)",
          fill: true,
          tension: 0.35,
          pointRadius: 0,
        },
        {
          label: "Queue",
          data: chartPoints.map((p) => p.queue),
          borderColor: "#f59e0b",
          backgroundColor: "rgba(245,158,11,0.08)",
          fill: true,
          tension: 0.35,
          pointRadius: 0,
        },
      ],
    }),
    []
  );

  const chartOptions: ChartOptions<"line"> = useMemo(
    () => ({
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          labels: { color: "#e5e7eb", usePointStyle: true },
        },
        tooltip: { intersect: false, mode: "index" },
      },
      scales: {
        x: {
          grid: { color: "rgba(255,255,255,0.06)" },
          ticks: { color: "#9ca3af" },
        },
        y: {
          grid: { color: "rgba(255,255,255,0.06)" },
          ticks: { color: "#9ca3af" },
        },
      },
    }),
    []
  );

  const renderTree = (nodes: FileNode[], depth = 0) =>
    nodes.map((node) => {
      const isFile = node.type === "file";
      const isActive = isFile && node.path === activePath;
      const isCollapsed = collapsed.has(node.path);
      return (
        <div className="tree-item" key={node.path}>
          <div
            className={`tree-row ${isActive ? "active" : ""}`}
            style={{ paddingLeft: depth * 10 + 6 }}
            onClick={() => {
              if (node.type === "file") {
                setActivePath(node.path);
              } else if (node.children) {
                const next = findFirstFile(node.children);
                if (next) setActivePath(next.path);
              }
            }}
          >
            <div className="node-head">
              <span className={`node-icon ${isFile ? "file" : "dir"}`} />
              <span className={`pill ${isFile ? "info" : "success"} mini-pill`}>{isFile ? "FILE" : "DIR"}</span>
            </div>
            <div className="node-body">
              <div className="node-title">{node.name}</div>
              <div className="node-sub">{node.path}</div>
              <div className="node-sub" style={{ opacity: 0.9 }}>
                {isFile ? `${node.size ?? "--"} · 更新 ${node.updated ?? "--"}` : `${node.children?.length ?? 0} 项 · 层级 ${depth + 1}`}
              </div>
            </div>
            <div className="node-actions">
              {node.type === "dir" ? (
                <button
                  className="collapse-btn"
                  onClick={(e) => {
                    e.stopPropagation();
                    handleCollapseToggle(node.path);
                  }}
                >
                  {isCollapsed ? "展开" : "收起"}
                </button>
              ) : null}
              <span className={`pill ${node.autoUpload ? "success" : "warning"}`}>{node.autoUpload ? "自动" : "手动"}</span>
              <label className="switch mini" onClick={(e) => e.stopPropagation()}>
                <input
                  type="checkbox"
                  checked={node.autoUpload}
                  onChange={(e) => handleAutoToggle(node.path, e.target.checked)}
                />
                <span className="slider" />
              </label>
            </div>
          </div>
          {node.children && !isCollapsed ? <div className="tree-children">{renderTree(node.children, depth + 1)}</div> : null}
        </div>
      );
    });

  const manualUploadTime = manualUploadMap[activePath];
  const updatedSegment = manualUploadTime
    ? `手动上传 ${fmt(manualUploadTime)}`
    : activeNode?.updated
    ? `更新 ${fmt(activeNode.updated)}`
    : "更新时间未知";

  return (
    <div className="page-shell">
      <div className="layout">
        <aside className="sidebar">
          <div className="nav-brand">
            <div className="brand-badge small">FW</div>
            <div>
              <div className="nav-title">File Watch</div>
              <div className="nav-sub">单机 Agent 控制台</div>
            </div>
          </div>
          <div className="nav-meta">
            <span className="pill success">运行中</span>
            <span className="badge">srv-01</span>
            <span className="badge">全量监控</span>
          </div>
          <nav className="nav-list">
            {SECTION_IDS.map((id) => (
              <a key={id} className={`nav-item ${activeSection === id ? "active" : ""}`} href={`#${id}`}>
                <span>
                  {id === "overview" ? "总览" : id === "directory" ? "目录浏览" : id === "files" ? "文件列表" : id === "tail" ? "Tail / 配置" : id === "failures" ? "失败重试" : "监控"}
                </span>
                <small>
                  {id === "overview"
                    ? "心跳 / 摘要"
                    : id === "directory"
                    ? "自动上传"
                    : id === "files"
                    ? "CRUD"
                    : id === "tail"
                    ? "实时"
                    : id === "failures"
                    ? "队列"
                    : "吞吐"}
                </small>
              </a>
            ))}
          </nav>
          <div className="nav-foot">
            <div className="nav-foot-row">
              <span>后缀过滤</span>
              <span className="pill warning">关闭</span>
            </div>
            <div className="nav-foot-row">
              <span>目录递归</span>
              <span className="pill success">开启</span>
            </div>
            <div className="nav-foot-row">
              <span>自动刷新</span>
              <label className="switch mini">
                <input type="checkbox" defaultChecked />
                <span className="slider" />
              </label>
            </div>
            <div className="nav-foot-row">
              <span>上传并发</span>
              <span className="badge">8 workers</span>
            </div>
          </div>
        </aside>

        <div className="page">
          <header className="page-header">
            <div className="brand">
              <div className="brand-badge">FW</div>
              <div className="title">
                <h1>单机文件监控 Agent 控制台</h1>
                <p>针对当前主机的目录监听、上云、路由与告警视图</p>
              </div>
            </div>
            <div className="controls">
              <div className="chip active">实时</div>
              <div className="chip">最近 24h</div>
              <button className="btn secondary" type="button">
                重载配置
              </button>
              <button className="btn" type="button">
                手动上传/同步
              </button>
            </div>
          </header>

          <div id="overview" className="stack" style={{ gap: 12 }}>
            <div className="hero">
              <div>
                <div className="hero-title">
                  当前 Agent：<strong>{heroCopy.agent}</strong>
                </div>
                <div className="hero-desc">
                  监听目录 <strong>{heroCopy.watchDirs.join(" , ")}</strong> ，后缀过滤{heroCopy.suffixFilter}；静默窗口 {heroCopy.silence}，{heroCopy.queue}，{heroCopy.concurrency}，目标存储 <strong>{heroCopy.bucket}</strong>。 单机视角：本地主机事件、队列、告警与上云进度一屏掌握，目录树可逐个文件关闭自动上传。
                </div>
              </div>
              <div className="hero-list">
                {heroHighlights.map((item) => (
                  <span key={item}>{item}</span>
                ))}
              </div>
            </div>

            <section className="grid">
              {metricCards.map((card) => (
                <div className="card" key={card.label}>
                  <small>{card.label}</small>
                  <div className="value">
                    {card.value}{" "}
                    <span className={`trend ${card.tone === "up" ? "up" : card.tone === "down" ? "down" : card.tone === "warning" ? "warning" : ""}`}>
                      {card.trend}
                    </span>
                  </div>
                </div>
              ))}
            </section>
          </div>

          <section className="panel" id="directory">
            <div className="section-title">
              <h2>目录浏览 / 自动上传控制</h2>
              <span>全量目录监控 · 后缀匹配已关闭</span>
            </div>
            <div className="workspace">
              <div className="tree-panel">
                <div className="toolbar">
                  <select
                    className="select"
                    value={currentRoot}
                    onChange={(e) => {
                      setCollapsed(new Set());
                      setCurrentRoot(e.target.value);
                    }}
                  >
                    {tree.map((node) => (
                      <option key={node.path} value={node.path}>
                        {node.path}
                      </option>
                    ))}
                  </select>
                  <div className="chip active">递归监听</div>
                  <div className="chip">仅新文件</div>
                  <span className="badge">自动刷新</span>
                  <button className="btn secondary" type="button" onClick={() => handleCollapseAll(true)}>
                    全部收起
                  </button>
                  <button className="btn secondary" type="button" onClick={() => handleCollapseAll(false)}>
                    全部展开
                  </button>
                </div>
                <div className="tree">{renderTree(rootNodes)}</div>
              </div>

              <div className="file-preview">
                <div className="preview-header">
                  <div>
                    <span className={`pill ${activeNode?.autoUpload ? "success" : "warning"}`} id="previewState">
                      {activeNode ? (activeNode.autoUpload ? "自动上传开启" : "自动上传关闭") : "选择一个文件"}
                    </span>
                    <div className="preview-title" id="previewTitle">
                      {activeNode?.name ?? "未选择文件"}
                    </div>
                    <div className="preview-path" id="previewPath">
                      {activeNode?.path ?? "—"}
                    </div>
                  </div>
                  <div className="switch-group">
                    <span className="muted">自动上传</span>
                    <label className="switch">
                      <input
                        type="checkbox"
                        checked={!!activeNode?.autoUpload}
                        onChange={(e) => activeNode && handleAutoToggle(activeNode.path, e.target.checked)}
                        disabled={!activeNode}
                      />
                      <span className="slider" />
                    </label>
                    <button className="btn secondary" type="button" onClick={handleManualUpload} disabled={!activeNode}>
                      立即上传
                    </button>
                  </div>
                </div>
                <div className="preview-meta" id="previewMeta">
                  {activeNode ? `大小 ${activeNode.size ?? "--"} · ${updatedSegment} · 目录监控中` : "目录树展示全部文件，可切换自动上传"}
                </div>
                <div className="preview-content" id="previewContent">
                  {activeNode ? activeNode.content ?? "文件内容预览" : "点击左侧目录树中的文件预览内容。"}
                </div>
              </div>
            </div>
          </section>

          <section className="panel" id="files">
            <div className="section-title">
              <h2>文件列表（单机 CRUD）</h2>
              <span>查看 / 下载 / 删除 / 重试</span>
            </div>
            <div className="toolbar">
              <input className="search" placeholder="搜索文件名 / 路径" value={searchTerm} onChange={(e) => setSearchTerm(e.target.value)} />
              <div className={`chip ${fileFilter === "all" ? "active" : ""}`} onClick={() => setFileFilter("all")}>
                全部
              </div>
              <div className={`chip ${fileFilter === "auto" ? "active" : ""}`} onClick={() => setFileFilter("auto")}>
                自动上传
              </div>
              <div className={`chip ${fileFilter === "manual" ? "active" : ""}`} onClick={() => setFileFilter("manual")}>
                手动上传
              </div>
              <div className={`chip ${fileFilter === "approval" ? "active" : ""}`} onClick={() => setFileFilter("approval")}>
                需审批
              </div>
              <div className={`chip ${fileFilter === "failed" ? "active" : ""}`} onClick={() => setFileFilter("failed")}>
                失败
              </div>
              <span className="badge">后缀过滤已关闭 · 按目录控制</span>
              <button className="btn secondary" type="button">
                批量删除
              </button>
            </div>
            <table className="table">
              <thead>
                <tr>
                  <th>文件</th>
                  <th>大小</th>
                  <th>自动上传</th>
                  <th>状态</th>
                  <th>更新时间</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filteredFiles.map((f) => (
                  <tr key={f.path}>
                    <td>
                      <div className="row-title">{f.name}</div>
                      <div className="row-sub">{f.path}</div>
                    </td>
                    <td>{f.size}</td>
                    <td>
                      <span className={`pill ${f.autoUpload ? "success" : "warning"}`}>{f.autoUpload ? "开启" : "关闭"}</span>
                    </td>
                    <td>
                      <span className="badge">
                        {f.status === "uploaded" ? "已上传" : f.status === "queued" ? "队列中" : "失败"}
                      </span>
                    </td>
                    <td>{fmt(f.time)}</td>
                    <td>
                      <div className="table-actions">
                        <button className="btn secondary" type="button">
                          查看
                        </button>
                        <button className="btn secondary" type="button">
                          下载
                        </button>
                        <button className="btn secondary" type="button">
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className="flex-2" id="tail">
            <div className="panel">
              <div className="section-title">
                <h2>实时 Tail / 关键字</h2>
                <span>最近 200 行</span>
              </div>
              <div className="toolbar">
                <div className="chip active">实时</div>
                <div className="chip">仅错误</div>
                <div className="chip">下载原文件</div>
              </div>
              <pre className="tail-box">{tailLines.join("\n")}</pre>
              <div className="timeline">
                {timelineEvents.map((ev) => (
                  <div className="timeline-item" key={`${ev.label}-${ev.time}`}>
                    <span className={`pill ${ev.status}`}>{ev.label}</span>
                    <div className="timeline-text">{fmt(ev.time)}</div>
                    <div className="timeline-text right">{ev.host ?? "srv-01"}</div>
                  </div>
                ))}
              </div>
            </div>

            <div className="panel">
              <div className="section-title">
                <h2>上传与路由配置（本机）</h2>
                <span>表单仅为演示</span>
              </div>
              <div className="inputs">
                <div className="input">
                  <label>watch_dir</label>
                  <input value={configSnapshot.watchDir} readOnly />
                </div>
                <div className="input">
                  <label>后缀过滤</label>
                  <input value={configSnapshot.fileExt} readOnly />
                </div>
                <div className="input">
                  <label>静默窗口</label>
                  <input value={configSnapshot.silence} readOnly />
                </div>
                <div className="input">
                  <label>并发/队列</label>
                  <input value={configSnapshot.concurrency} readOnly />
                </div>
                <div className="input">
                  <label>S3 目标</label>
                  <input value={configSnapshot.bucket} readOnly />
                </div>
                <div className="input">
                  <label>目录策略</label>
                  <input value={configSnapshot.strategy} readOnly />
                </div>
                <div className="input">
                  <label>Action</label>
                  <select value={configSnapshot.action} disabled>
                    <option>上传 + Webhook</option>
                    <option>上传 + 队列</option>
                    <option>隔离 + 审核</option>
                  </select>
                </div>
              </div>
              <div className="toolbar space-between">
                <span className="badge">路径校验已启用 · 防穿越</span>
                <button className="btn" type="button">
                  预览路由
                </button>
              </div>
              <div className="stack">
                {routes.map((r) => (
                  <div className="stack-item" key={r.name}>
                    <strong>{r.name}</strong>
                    <div className="meta">When: {r.cond}</div>
                    <div className="meta">Action: {r.action}</div>
                  </div>
                ))}
              </div>
            </div>
          </section>

          <section className="panel" id="failures">
            <div className="section-title">
              <h2>失败 / 重试 / 告警</h2>
              <span>单机队列</span>
            </div>
            <table className="table">
              <thead>
                <tr>
                  <th>文件</th>
                  <th>原因</th>
                  <th>尝试</th>
                  <th>下一步</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {failures.map((f) => (
                  <tr key={f.file}>
                    <td>{f.file}</td>
                    <td>
                      <span className="badge">{f.reason}</span>
                    </td>
                    <td>{f.attempts}x</td>
                    <td>{f.next}</td>
                    <td>
                      <button className="btn secondary" type="button">
                        重试
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>

          <section className="panel" id="monitor">
            <div className="section-title">
              <h2>运行与上云监控</h2>
              <span>吞吐 / 队列 / 时延</span>
            </div>
            <div className="flex-2 stretch">
              <div className="chart-wrapper">
                <Line data={chartData} options={chartOptions} />
              </div>
              <div className="stack">
                {monitorNotes.map((note) => (
                  <div className="stack-item" key={note.title}>
                    <strong>{note.title}</strong>
                    <div className="meta">{note.detail}</div>
                  </div>
                ))}
              </div>
            </div>
          </section>
        </div>
      </div>
    </div>
  );
}

export default App;

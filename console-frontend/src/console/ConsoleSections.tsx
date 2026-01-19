import type { ReactNode, RefObject } from "react";
import { Line } from "react-chartjs-2";
import type { ChartData, ChartOptions } from "chart.js";
import type {
  ConfigSnapshot,
  FileFilter,
  FileItem,
  FileNode,
  HeroCopy,
  MetricCard,
  MonitorNote,
  MonitorSummary,
  UploadRecord,
} from "../types";
import type { LogMode } from "./dashboardApi";

type OverviewSectionProps = {
  metricCards: MetricCard[];
  hero: HeroCopy;
  silenceValue: string;
};

export function OverviewSection({ metricCards, hero, silenceValue }: OverviewSectionProps) {
  return (
    <div id="overview" className="stack" style={{ gap: 12 }}>
      <section className="metric-strip">
        {metricCards.map((card) => (
          <div className="metric-tile" key={card.label}>
            <small>{card.label}</small>
            <div className="value">
              {card.value}{" "}
              <span
                className={`trend ${card.tone === "up" ? "up" : card.tone === "down" ? "down" : card.tone === "warning" ? "warning" : ""}`}
              >
                {card.trend}
              </span>
            </div>
          </div>
        ))}
      </section>

      <div className="hero hero-plain">
        <div className="hero-left">
          <div className="hero-status">
            <span className="pill success">运行中</span>
            <span className="badge ghost">Agent {hero.agent}</span>
            <span className="badge ghost">监听 {hero.watchDirs.length} 目录</span>
          </div>
          <div className="hero-desc">
            针对当前主机的目录监听、上云路由与告警视图，核心状态收敛在下方卡片；目录树与文件列表用于日常巡检。
          </div>
        </div>
        <div className="hero-right">
          <div className="hero-right-grid">
            <div className="stat-compact">
              <small>监听目录</small>
              <div className="hero-tags">
                {hero.watchDirs.map((dir) => (
                  <span className="hero-tag" key={dir} title={dir}>
                    {dir}
                  </span>
                ))}
              </div>
            </div>
            <div className="stat-compact">
              <small>后缀过滤</small>
              <strong>{hero.suffixFilter}</strong>
            </div>
            <div className="stat-compact">
              <small>静默窗口</small>
              <strong>{silenceValue}</strong>
            </div>
            <div className="stat-compact">
              <small>并发数量</small>
              <strong>{hero.concurrency}</strong>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

type ConfigSectionProps = {
  configForm: ConfigSnapshot;
  saving: boolean;
  saveMessage: string | null;
  onChange: (patch: Partial<ConfigSnapshot>) => void;
  onSave: () => void;
};

export function ConfigSection({ configForm, saving, saveMessage, onChange, onSave }: ConfigSectionProps) {
  return (
    <section className="panel" id="config">
      <div className="section-title">
        <h2>上传与路由配置</h2>
      </div>
      <div className="inputs">
        <div className="input">
          <label>监控目录</label>
          <input
            placeholder={'Enter directories separated by space/comma/semicolon; quote paths with spaces (e.g. "/data/my logs" /data/other)'}
            value={configForm.watchDir}
            onChange={(e) => onChange({ watchDir: e.target.value })}
          />
        </div>
        <div className="input">
          <label>文件后缀过滤</label>
          <input
            placeholder="支持多个后缀（逗号分隔），不填则默认显示监控目录下的所有文件和子目录"
            value={configForm.fileExt}
            onChange={(e) => onChange({ fileExt: e.target.value })}
          />
        </div>
        <div className="input">
          <label>静默窗口</label>
          <input placeholder="例如 10s / 30s" value={configForm.silence} onChange={(e) => onChange({ silence: e.target.value })} />
        </div>
        <div className="input">
          <label>并发/队列</label>
          <input
            placeholder="示例：workers=3 / queue=100（必填，数字）"
            value={configForm.concurrency}
            onChange={(e) => onChange({ concurrency: e.target.value })}
          />
        </div>
      </div>
      <div className="toolbar config-actions">
        <div className="toolbar-actions">
          <button className="btn" type="button" onClick={onSave} disabled={saving}>
            {saving ? "保存中..." : "保存配置"}
          </button>
        </div>
      </div>
      {saveMessage ? <div className="badge">{saveMessage}</div> : null}
    </section>
  );
}

type DirectorySectionProps = {
  tree: FileNode[];
  currentRoot: string;
  treeFilesCount: number;
  treeFilterBadge: string;
  rootNodes: FileNode[];
  activeNode?: FileNode;
  updatedTime: string;
  onRootChange: (nextRoot: string) => void;
  onCollapseAll: (collapse: boolean) => void;
  renderTree: (nodes: FileNode[], depth?: number) => ReactNode;
  onAutoToggle: (path: string, enabled: boolean) => void;
  onManualUpload: () => void;
};

export function DirectorySection({
  tree,
  currentRoot,
  treeFilesCount,
  treeFilterBadge,
  rootNodes,
  activeNode,
  updatedTime,
  onRootChange,
  onCollapseAll,
  renderTree,
  onAutoToggle,
  onManualUpload,
}: DirectorySectionProps) {
  return (
    <section className="panel" id="directory">
      <div className="section-title">
        <h2>目录浏览 / 自动上传控制</h2>
      </div>
      <div className="workspace">
        <div className="tree-panel">
          <div className="toolbar">
            <select className="select" value={currentRoot} onChange={(e) => onRootChange(e.target.value)}>
              {tree.map((node) => (
                <option key={node.path} value={node.path}>
                  {node.path}
                </option>
              ))}
            </select>
            <button className="btn secondary" type="button" onClick={() => onCollapseAll(true)}>
              全部收起
            </button>
            <button className="btn secondary" type="button" onClick={() => onCollapseAll(false)}>
              全部展开
            </button>
          </div>
          <div className="tree-meta">
            <span className="badge ghost">{treeFilterBadge}</span>
            <span className="badge ghost">匹配文件: {treeFilesCount}</span>
          </div>
          {treeFilesCount === 0 ? <div className="empty-state">当前过滤下暂无匹配文件</div> : null}
          <div className="tree">{renderTree(rootNodes)}</div>
        </div>

        <div className="file-preview">
          <div className="preview-header">
            <div className="preview-main">
              <span className={`pill compact ${activeNode?.autoUpload ? "success" : "warning"}`} id="previewState">
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
                  onChange={(e) => activeNode && onAutoToggle(activeNode.path, e.target.checked)}
                  disabled={!activeNode}
                />
                <span className="slider" />
              </label>
              <button className="btn secondary" type="button" onClick={onManualUpload} disabled={!activeNode}>
                立即上传
              </button>
            </div>
          </div>
          <div className="info-grid">
            <div className="info-item">
              <span className="muted">大小</span>
              <strong>{activeNode?.size ?? "--"}</strong>
            </div>
            <div className="info-item">
              <span className="muted">更新时间</span>
              <strong>{updatedTime}</strong>
            </div>
            <div className="info-item">
              <span className="muted">模式</span>
              <strong>{activeNode ? (activeNode.autoUpload ? "自动上传" : "手动上传") : "--"}</strong>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

type FilesSectionProps = {
  searchTerm: string;
  fileFilter: FileFilter;
  actionMessage: string | null;
  filesPage: FileItem[];
  page: number;
  pageCount: number;
  onSearchChange: (value: string) => void;
  onFileFilterChange: (value: FileFilter) => void;
  onPageChange: (next: number) => void;
  onViewLog: (file: FileItem) => void;
  onDownloadFile: (file: FileItem) => void;
  formatTime: (value: string) => string;
};

export function FilesSection({
  searchTerm,
  fileFilter,
  actionMessage,
  filesPage,
  page,
  pageCount,
  onSearchChange,
  onFileFilterChange,
  onPageChange,
  onViewLog,
  onDownloadFile,
  formatTime,
}: FilesSectionProps) {
  return (
    <section className="panel" id="files">
      <div className="section-title">
        <h2>文件列表</h2>
      </div>
      <div className="toolbar">
        <input className="search" placeholder="搜索文件名 / 路径" value={searchTerm} onChange={(e) => onSearchChange(e.target.value)} />
        <div className={`chip ${fileFilter === "all" ? "active" : ""}`} onClick={() => onFileFilterChange("all")}>
          全部
        </div>
        <div className={`chip ${fileFilter === "auto" ? "active" : ""}`} onClick={() => onFileFilterChange("auto")}>
          自动上传
        </div>
        <div className={`chip ${fileFilter === "manual" ? "active" : ""}`} onClick={() => onFileFilterChange("manual")}>
          手动上传
        </div>
        {actionMessage ? <span className="badge ghost">{actionMessage}</span> : null}
      </div>
      <table className="table files-table">
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
          {filesPage.length ? (
            filesPage.map((f) => (
              <tr key={f.path}>
                <td>
                  <div className="row-title">{f.name}</div>
                  <div className="row-sub" title={f.path}>
                    {f.path}
                  </div>
                </td>
                <td>{f.size}</td>
                <td>
                  <span className={`pill table-pill ${f.autoUpload ? "success" : "warning"}`}>{f.autoUpload ? "开启" : "关闭"}</span>
                </td>
                <td>
                  <span className="badge">
                    {f.status === "uploaded"
                      ? "已上传"
                      : f.status === "queued"
                        ? "队列中"
                        : f.status === "existing"
                          ? "已存在"
                          : "失败"}
                  </span>
                </td>
                <td>{formatTime(f.time)}</td>
                <td>
                  <div className="table-actions">
                    <button className="btn secondary" type="button" onClick={() => onViewLog(f)}>
                      查看
                    </button>
                    <button className="btn secondary" type="button" onClick={() => onDownloadFile(f)}>
                      下载
                    </button>
                  </div>
                </td>
              </tr>
            ))
          ) : (
            <tr>
              <td className="table-empty" colSpan={6}>
                暂无匹配文件
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="pagination">
        <button className="btn secondary" type="button" disabled={page <= 1} onClick={() => onPageChange(Math.max(1, page - 1))}>
          上一页
        </button>
        <span className="badge ghost">
          第 {page} / {pageCount} 页
        </span>
        <button
          className="btn secondary"
          type="button"
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          下一页
        </button>
      </div>
    </section>
  );
}

type TailSectionProps = {
  logMode: LogMode;
  logQuery: string;
  logQueryApplied: string;
  logTruncated: boolean;
  tailLines: string[];
  canSearch: boolean;
  tailBoxRef: RefObject<HTMLDivElement | null>;
  onSwitchTail: () => void;
  onRunSearch: () => void;
  onLogQueryChange: (value: string) => void;
  onClear: () => void;
  onScroll: () => void;
  renderLogLine: (line: string) => ReactNode;
};

export function TailSection({
  logMode,
  logQuery,
  logQueryApplied,
  logTruncated,
  tailLines,
  canSearch,
  tailBoxRef,
  onSwitchTail,
  onRunSearch,
  onLogQueryChange,
  onClear,
  onScroll,
  renderLogLine,
}: TailSectionProps) {
  return (
    <section className="panel" id="tail">
      <div className="section-title">
        <h2>文件内容</h2>
      </div>
      <div className="toolbar space-between">
        <div className="toolbar-actions">
          <div className={`chip ${logMode === "tail" ? "active" : ""}`} onClick={onSwitchTail}>
            实时
          </div>
          <div className={`chip ${logMode === "search" ? "active" : ""}`} onClick={onRunSearch}>
            全文检索
          </div>
          {logMode === "search" && logQueryApplied ? (
            <span className="badge ghost">
              关键词 {logQueryApplied} · 匹配 {tailLines.length} 行{logTruncated ? " · 已截断" : ""}
            </span>
          ) : null}
        </div>
        <div className="toolbar-actions">
          <input
            className="search log-search"
            placeholder="关键词/全文检索"
            value={logQuery}
            onChange={(e) => onLogQueryChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") onRunSearch();
            }}
          />
          <button className="btn secondary" type="button" onClick={onRunSearch} disabled={!canSearch}>
            检索
          </button>
          <button className="btn secondary" type="button" onClick={onClear}>
            清除
          </button>
        </div>
      </div>
      <div className="tail-box" ref={tailBoxRef} onScroll={onScroll}>
        {logMode === "search" && logQueryApplied && tailLines.length === 0 ? (
          <div className="tail-line">未找到匹配内容</div>
        ) : (
          tailLines.map((line, idx) => (
            <div className="tail-line" key={`${line}-${idx}`}>
              {renderLogLine(line)}
            </div>
          ))
        )}
      </div>
    </section>
  );
}

type UploadsSectionProps = {
  uploadSearchTerm: string;
  records: UploadRecord[];
  page: number;
  pageCount: number;
  onUploadSearchChange: (value: string) => void;
  onPageChange: (next: number) => void;
  formatTime: (value: string) => string;
};

export function UploadsSection({
  uploadSearchTerm,
  records,
  page,
  pageCount,
  onUploadSearchChange,
  onPageChange,
  formatTime,
}: UploadsSectionProps) {
  return (
    <section className="panel" id="failures">
      <div className="section-title">
        <h2>上传记录 / 最近动作</h2>
        <input
          className="search"
          placeholder="搜索状态：成功 / 失败 / 排队"
          value={uploadSearchTerm}
          onChange={(e) => onUploadSearchChange(e.target.value)}
        />
      </div>
      <table className="table upload-table">
        <thead>
          <tr>
            <th>文件</th>
            <th>状态</th>
            <th>耗时</th>
            <th>下载地址</th>
            <th>时间</th>
            <th>备注</th>
          </tr>
        </thead>
        <tbody key={`${page}-${uploadSearchTerm}`}>
          {records.length ? (
            records.map((item, index) => (
              <tr key={`${item.file}-${item.time}-${item.result}-${index}`}>
                <td>
                  <div className="row-title">{item.file}</div>
                  <div className="row-sub">{item.size}</div>
                </td>
                <td>
                  <span className={`pill ${item.result === "success" ? "success" : item.result === "failed" ? "danger" : "warning"}`}>
                    {item.result === "success" ? "成功" : item.result === "failed" ? "失败" : "等待"}
                  </span>
                </td>
                <td>{item.latency}</td>
                <td className="upload-target" title={item.target ?? ""}>
                  {item.target || "--"}
                </td>
                <td>{formatTime(item.time)}</td>
                <td className="upload-note">
                  <div className="row-sub">{item.note ?? "--"}</div>
                </td>
              </tr>
            ))
          ) : (
            <tr>
              <td className="table-empty" colSpan={6}>
                暂无匹配记录
              </td>
            </tr>
          )}
        </tbody>
      </table>
      <div className="pagination">
        <button className="btn secondary" type="button" disabled={page <= 1} onClick={() => onPageChange(Math.max(1, page - 1))}>
          上一页
        </button>
        <span className="badge ghost">
          第 {page} / {pageCount} 页
        </span>
        <button
          className="btn secondary"
          type="button"
          disabled={page >= pageCount}
          onClick={() => onPageChange(Math.min(pageCount, page + 1))}
        >
          下一页
        </button>
      </div>
    </section>
  );
}

type MonitorSectionProps = {
  summary: MonitorSummary[];
  notes: MonitorNote[];
  chartData: ChartData<"line">;
  chartOptions: ChartOptions<"line">;
};

export function MonitorSection({ summary, notes, chartData, chartOptions }: MonitorSectionProps) {
  return (
    <section className="panel" id="monitor">
      <div className="section-title">
        <h2>运行与上云监控</h2>
        <span>吞吐 / 队列 / 时延</span>
      </div>
      <div className="monitor-grid">
        {summary.map((item) => (
          <div className="card compact" key={item.label}>
            <div className="value">{item.value}</div>
            <div className="meta">{item.label}</div>
            <div className="muted small">{item.desc}</div>
          </div>
        ))}
      </div>
      <div className="flex-2 stretch">
        <div className="chart-wrapper">
          <Line data={chartData} options={chartOptions} />
        </div>
        <div className="stack">
          {notes.map((note) => (
            <div className="stack-item" key={note.title}>
              <strong>{note.title}</strong>
              <div className="meta">{note.detail}</div>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

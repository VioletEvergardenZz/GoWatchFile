type ConsoleHeaderProps = {
  agent: string;
  loading: boolean;
  error: string | null;
  timeframe: "realtime" | "24h";
  onTimeframeChange: (value: "realtime" | "24h") => void;
  theme: "dark" | "light";
  onThemeChange: (value: "dark" | "light") => void;
};

export function ConsoleHeader({
  agent,
  loading,
  error,
  timeframe,
  onTimeframeChange,
  theme,
  onThemeChange,
}: ConsoleHeaderProps) {
  return (
    <header className="page-header">
      <div className="brand">
        <div className="title">
          <p className="eyebrow">Agent 控制台</p>
          <h1>文件监控 Agent 控制台</h1>
          <div className="title-meta">
            <span className="badge ghost">主机 {agent}</span>
          </div>
        </div>
      </div>
      <div className="controls">
        {loading ? <span className="badge">刷新中</span> : null}
        {error ? (
          <>
            <span className="pill danger">接口异常</span>
            <span className="badge ghost">{error}</span>
          </>
        ) : null}
        <div className={`chip ${timeframe === "realtime" ? "active" : ""}`} onClick={() => onTimeframeChange("realtime")}>
          实时
        </div>
        <div className="theme-toggle">
          <span className="muted small">背景</span>
          <label className="switch mini">
            <input
              type="checkbox"
              aria-label="切换深色/浅色背景"
              checked={theme === "light"}
              onChange={(e) => onThemeChange(e.target.checked ? "light" : "dark")}
            />
            <span className="slider" />
          </label>
          <span className="badge ghost">{theme === "light" ? "浅色" : "深色"}</span>
        </div>
      </div>
    </header>
  );
}

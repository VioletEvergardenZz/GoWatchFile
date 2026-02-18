/**
 * 文件职责：承载当前页面或模块的核心交互与状态管理
 * 关键交互：先更新本地状态 再调用接口同步 失败时给出可见反馈
 * 边界处理：对空数据 异常数据和超时请求提供兜底展示
 */

/* 本文件用于主控制台头部组件 集中处理状态栏和快捷操作区 */

type ConsoleHeaderProps = {
  agent: string;
  loading: boolean;
  error: string | null;
  timeframe: "realtime" | "24h";
  onTimeframeChange: (value: "realtime" | "24h") => void;
  theme: "dark" | "light";
  onThemeChange: (value: "dark" | "light") => void;
  apiToken: string;
  tokenRemember: boolean;
  tokenApplied: boolean;
  tokenSaving: boolean;
  onApiTokenChange: (value: string) => void;
  onTokenRememberChange: (value: boolean) => void;
  onSaveApiToken: () => void;
  onClearApiToken: () => void;
};

export function ConsoleHeader({
  agent,
  loading,
  error,
  timeframe,
  onTimeframeChange,
  theme,
  onThemeChange,
  apiToken,
  tokenRemember,
  tokenApplied,
  tokenSaving,
  onApiTokenChange,
  onTokenRememberChange,
  onSaveApiToken,
  onClearApiToken,
}: ConsoleHeaderProps) {
  return (
    <header className="page-header">
      <div className="brand">
        <div className="title">
          <p className="eyebrow">Agent Console</p>
          <h1>File Watch Agent Console</h1>
          <div className="title-meta">
            <span className="badge ghost">Host {agent}</span>
          </div>
        </div>
      </div>
      <div className="controls">
        {loading ? <span className="badge">Refreshing...</span> : null}
        {error ? (
          <>
            <span className="pill danger">API Error</span>
            <span className="badge ghost">{error}</span>
          </>
        ) : null}
        <div className={`chip ${timeframe === "realtime" ? "active" : ""}`} onClick={() => onTimeframeChange("realtime")}>
          Realtime
        </div>
        <div className="theme-toggle">
          <span className="muted small">Theme</span>
          <label className="switch mini">
            <input
              type="checkbox"
              aria-label="Toggle light and dark themes"
              checked={theme === "light"}
              onChange={(e) => onThemeChange(e.target.checked ? "light" : "dark")}
            />
            <span className="slider" />
          </label>
          <span className="badge ghost">{theme === "light" ? "Light" : "Dark"}</span>
        </div>
        <div className="api-token-panel">
          <input
            className="input api-token-input"
            type="password"
            autoComplete="off"
            spellCheck={false}
            placeholder="API Token"
            value={apiToken}
            onChange={(e) => onApiTokenChange(e.target.value)}
          />
          <label className="token-remember">
            <input type="checkbox" checked={tokenRemember} onChange={(e) => onTokenRememberChange(e.target.checked)} />
            <span>Remember</span>
          </label>
          <button className="btn secondary btn-token" type="button" onClick={onSaveApiToken} disabled={tokenSaving}>
            {tokenSaving ? "Saving..." : tokenApplied ? "Update Token" : "Set Token"}
          </button>
          <button className="btn secondary btn-token-clear" type="button" onClick={onClearApiToken}>
            Clear
          </button>
        </div>
      </div>
    </header>
  );
}


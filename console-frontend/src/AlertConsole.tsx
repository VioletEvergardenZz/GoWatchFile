import { useEffect, useMemo, useRef, useState } from "react";
import "./Alert.css";
import { alertConfigSnapshot, alertDashboard } from "./mockData";
import type {
  AlertConfigResponse,
  AlertConfigSnapshot,
  AlertDashboard,
  AlertDecisionStatus,
  AlertLevel,
  AlertResponse,
} from "./types";

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";
const USE_MOCK = ((import.meta.env.VITE_USE_MOCK as string | undefined) ?? "").toLowerCase() === "true";
const POLL_MS = 3000;
const DECISIONS_PAGE_SIZE = 5;

const emptyDashboard: AlertDashboard = {
  overview: {
    window: "--",
    risk: "--",
    fatal: 0,
    system: 0,
    business: 0,
    sent: 0,
    suppressed: 0,
    latest: "--",
  },
  decisions: [],
  stats: {
    sent: 0,
    suppressed: 0,
    recorded: 0,
  },
  rules: {
    source: "--",
    lastLoaded: "--",
    total: 0,
    defaultSuppress: "--",
    escalation: "--",
    levels: {
      ignore: 0,
      business: 0,
      system: 0,
      fatal: 0,
    },
  },
  polling: {
    interval: "--",
    logFiles: [],
    lastPoll: "--",
    nextPoll: "--",
  },
};

const emptyAlertConfig: AlertConfigSnapshot = {
  enabled: false,
  suppressEnabled: true,
  rulesFile: "",
  logPaths: "",
  pollInterval: "2s",
  startFromEnd: true,
};

const LEVEL_LABELS: Record<AlertLevel, string> = {
  ignore: "忽略",
  business: "业务",
  system: "系统",
  fatal: "致命",
};

const STATUS_LABELS: Record<AlertDecisionStatus, string> = {
  sent: "已发送",
  suppressed: "已抑制",
  recorded: "仅记录",
};

const resolveRiskTone = (risk: string) => {
  if (risk.includes("严重")) return "critical";
  if (risk.includes("高")) return "high";
  if (risk.includes("中")) return "medium";
  if (risk.includes("低")) return "low";
  return "muted";
};

const resolveLevelTone = (level: AlertLevel) => {
  if (level === "fatal") return "level-fatal";
  if (level === "system") return "level-system";
  if (level === "business") return "level-business";
  return "level-ignore";
};

const resolveStatusTone = (status: AlertDecisionStatus) => {
  if (status === "sent") return "status-sent";
  if (status === "suppressed") return "status-suppressed";
  return "status-recorded";
};

const formatTime = (value: string) => (value && value !== "--" ? value : "--");

type AlertConsoleProps = {
  embedded?: boolean;
};

export function AlertConsole({ embedded = false }: AlertConsoleProps) {
  const [dashboard, setDashboard] = useState<AlertDashboard>(USE_MOCK ? alertDashboard : emptyDashboard);
  const [loading, setLoading] = useState(!USE_MOCK);
  const [error, setError] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(true);
  const [lastUpdated, setLastUpdated] = useState(() =>
    USE_MOCK ? new Date().toLocaleTimeString("zh-CN", { hour12: false }) : "--"
  );
  const [alertConfig, setAlertConfig] = useState<AlertConfigSnapshot>(USE_MOCK ? alertConfigSnapshot : emptyAlertConfig);
  const [configLoading, setConfigLoading] = useState(!USE_MOCK);
  const [configSaving, setConfigSaving] = useState(false);
  const [configMessage, setConfigMessage] = useState<string | null>(null);
  const [configError, setConfigError] = useState<string | null>(null);
  const [decisionPage, setDecisionPage] = useState(1);
  const fetchingRef = useRef(false);
  const configFetchingRef = useRef(false);
  const aliveRef = useRef(true);

  useEffect(() => {
    aliveRef.current = true;
    return () => {
      aliveRef.current = false;
    };
  }, []);

  const refreshDashboard = async () => {
    if (USE_MOCK || fetchingRef.current || !aliveRef.current) return;
    fetchingRef.current = true;
    try {
      const resp = await fetch(`${API_BASE}/api/alerts`, { cache: "no-store" });
      if (!resp.ok) {
        throw new Error(`接口异常 ${resp.status}`);
      }
      const payload = (await resp.json()) as AlertResponse;
      if (!payload.ok || !payload.data) {
        if (!aliveRef.current) return;
        setEnabled(false);
        setDashboard(emptyDashboard);
        setError(payload.error ?? "告警未启用");
        return;
      }
      if (!aliveRef.current) return;
      setEnabled(payload.enabled ?? true);
      setDashboard(payload.data);
      setError(null);
      setLastUpdated(new Date().toLocaleTimeString("zh-CN", { hour12: false }));
    } catch (err) {
      if (!aliveRef.current) return;
      const msg = err instanceof Error ? err.message : "获取告警失败";
      setError(msg);
    } finally {
      fetchingRef.current = false;
      if (aliveRef.current) {
        setLoading(false);
      }
    }
  };

  useEffect(() => {
    if (USE_MOCK) return;
    void refreshDashboard();
    const timer = window.setInterval(() => void refreshDashboard(), POLL_MS);
    return () => {
      window.clearInterval(timer);
    };
  }, []);

  useEffect(() => {
    if (USE_MOCK) return;
    const fetchConfig = async () => {
      if (configFetchingRef.current || !aliveRef.current) return;
      configFetchingRef.current = true;
      setConfigLoading(true);
      setConfigError(null);
      try {
        const resp = await fetch(`${API_BASE}/api/alert-config`, { cache: "no-store" });
        if (!resp.ok) {
          throw new Error(`接口异常 ${resp.status}`);
        }
        const payload = (await resp.json()) as AlertConfigResponse;
        if (payload.config && aliveRef.current) {
          setAlertConfig(payload.config);
        }
      } catch (err) {
        if (!aliveRef.current) return;
        const msg = err instanceof Error ? err.message : "获取配置失败";
        setConfigError(msg);
      } finally {
        configFetchingRef.current = false;
        if (aliveRef.current) {
          setConfigLoading(false);
        }
      }
    };
    void fetchConfig();
  }, []);

  const handleConfigSave = async () => {
    setConfigMessage(null);
    setConfigError(null);
    const rulesFile = alertConfig.rulesFile.trim();
    const logPaths = alertConfig.logPaths.trim();
    if (alertConfig.enabled) {
      if (!rulesFile) {
        setConfigError("请填写告警规则文件路径");
        return;
      }
      if (!logPaths) {
        setConfigError("请填写告警日志路径");
        return;
      }
    }
    setConfigSaving(true);
    try {
      const resp = await fetch(`${API_BASE}/api/alert-config`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          enabled: alertConfig.enabled,
          suppressEnabled: alertConfig.suppressEnabled,
          rulesFile: alertConfig.rulesFile,
          logPaths: alertConfig.logPaths,
          pollInterval: alertConfig.pollInterval,
          startFromEnd: alertConfig.startFromEnd,
        }),
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(text || `保存失败，状态码 ${resp.status}`);
      }
      const payload = (await resp.json()) as AlertConfigResponse;
      if (payload.config) {
        setAlertConfig(payload.config);
      }
      setConfigMessage("配置已更新并立即生效");
      await refreshDashboard();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "保存配置失败";
      setConfigError(msg);
    } finally {
      setConfigSaving(false);
    }
  };

  const overview = dashboard.overview;
  const stats = dashboard.stats;
  const rules = dashboard.rules;
  const polling = dashboard.polling;
  const decisions = dashboard.decisions ?? [];

  const totalSignals = stats.sent + stats.suppressed;
  const suppressionRate = totalSignals > 0 ? Math.round((stats.suppressed / totalSignals) * 100) : 0;
  const suppressionLabel = totalSignals > 0 ? `${suppressionRate}%` : "--";
  const sentPercent = totalSignals > 0 ? (stats.sent / totalSignals) * 100 : 0;
  const suppressedPercent = totalSignals > 0 ? (stats.suppressed / totalSignals) * 100 : 0;

  const riskLabel = enabled ? overview.risk : "未启用";
  const riskTone = resolveRiskTone(riskLabel);

  const summaryMetrics = useMemo(
    () => [
      { label: "致命", value: overview.fatal, tone: "metric-fatal" },
      { label: "系统", value: overview.system, tone: "metric-system" },
      { label: "业务", value: overview.business, tone: "metric-business" },
    ],
    [overview.business, overview.fatal, overview.system]
  );

  const configStatusLabel = configLoading ? "配置加载中" : alertConfig.enabled ? "已启用" : "已停用";
  const configDisabled = configLoading || configSaving;
  const totalDecisionPages = Math.max(1, Math.ceil(decisions.length / DECISIONS_PAGE_SIZE));
  const pagedDecisions = decisions.slice(
    (decisionPage - 1) * DECISIONS_PAGE_SIZE,
    decisionPage * DECISIONS_PAGE_SIZE
  );
  const hasPagination = decisions.length > DECISIONS_PAGE_SIZE;

  useEffect(() => {
    if (decisionPage > totalDecisionPages) {
      setDecisionPage(totalDecisionPages);
    } else if (decisionPage < 1) {
      setDecisionPage(1);
    }
  }, [decisionPage, totalDecisionPages]);

  return (
    <div className={`alert-shell${embedded ? " alert-embedded" : ""}`}>
      <header className="alert-header">
        <div className="header-copy">
          <p className="eyebrow">智能告警中间层</p>
          <h1>告警决策控制台</h1>
          <p className="subtitle">聚焦关键异常与抑制效果</p>
        </div>
        <div className="header-meta">
          <span className={`risk-pill risk-${riskTone}`}>{riskLabel}</span>
          <div className="meta-stack">
            <span>窗口 {overview.window}</span>
            <span>刷新 {lastUpdated}</span>
          </div>
          <span className={`live-dot ${loading ? "loading" : "active"}`} />
        </div>
      </header>

      {error ? <div className="alert-banner">{error}</div> : null}

      <section className="overview-grid">
        <div className="card overview-card">
          <div className="overview-top">
            <div>
              <p className="muted">告警态势概览</p>
              <h2 className="overview-title">风险等级 {riskLabel}</h2>
              <p className="muted">最新告警 {formatTime(overview.latest)}</p>
            </div>
            <div className={`risk-badge risk-${riskTone}`}>{riskLabel}</div>
          </div>
          <div className="overview-metrics">
            {summaryMetrics.map((item) => (
              <div className={`metric-block ${item.tone}`} key={item.label}>
                <span>{item.label}</span>
                <strong>{item.value}</strong>
              </div>
            ))}
          </div>
          <div className="overview-foot">
            <div>
              <span>已发送</span>
              <strong>{overview.sent}</strong>
            </div>
            <div>
              <span>已抑制</span>
              <strong>{overview.suppressed}</strong>
            </div>
            <div>
              <span>最新窗口</span>
              <strong>{overview.window}</strong>
            </div>
          </div>
        </div>

        <div className="card stats-card">
          <div className="stats-head">
            <div>
              <p className="muted">告警统计</p>
              <h3>发送 vs 抑制</h3>
            </div>
            <div className="stats-rate">
              <span>抑制率</span>
              <strong>{suppressionLabel}</strong>
            </div>
          </div>
          <div className="stats-bars">
            <div className="progress-track">
              <div className="progress-seg sent" style={{ width: `${sentPercent}%` }} />
              <div className="progress-seg suppressed" style={{ width: `${suppressedPercent}%` }} />
            </div>
            <div className="stats-legend">
              <div>
                <span className="dot sent" />
                发送 {stats.sent}
              </div>
              <div>
                <span className="dot suppressed" />
                抑制 {stats.suppressed}
              </div>
              <div>
                <span className="dot recorded" />
                仅记录 {stats.recorded}
              </div>
            </div>
          </div>
          <div className="stats-footer">
            <div>
              <span>决策总量</span>
              <strong>{stats.sent + stats.suppressed + stats.recorded}</strong>
            </div>
            <div>
              <span>抑制节省</span>
              <strong>{stats.suppressed}</strong>
            </div>
          </div>
        </div>
      </section>

      <section className="card alert-config-card">
        <div className="section-header">
          <div>
            <h2>告警配置</h2>
            <p className="muted">Hot reload; persisted to config.runtime.yaml.</p>
          </div>
          <div className="section-meta">{configStatusLabel}</div>
        </div>
        <div className="alert-config-grid">
          <div className="inputs config-switches">
            <div className="input">
              <label>告警开关</label>
              <div className="switch-group">
                <span className="muted small">{alertConfig.enabled ? "启用" : "停用"}</span>
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={alertConfig.enabled}
                    disabled={configDisabled}
                    onChange={(e) => setAlertConfig((prev) => ({ ...prev, enabled: e.target.checked }))}
                  />
                  <span className="slider" />
                </label>
              </div>
            </div>
            <div className="input">
              <label>告警抑制</label>
              <div className="switch-group">
                <span className="muted small">{alertConfig.suppressEnabled ? "开启" : "关闭"}</span>
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={alertConfig.suppressEnabled}
                    disabled={configDisabled}
                    onChange={(e) => setAlertConfig((prev) => ({ ...prev, suppressEnabled: e.target.checked }))}
                  />
                  <span className="slider" />
                </label>
              </div>
            </div>
            <div className="input">
              <label>从末尾开始</label>
              <div className="switch-group">
                <span className="muted small">{alertConfig.startFromEnd ? "是" : "否"}</span>
                <label className="switch">
                  <input
                    type="checkbox"
                    checked={alertConfig.startFromEnd}
                    disabled={configDisabled}
                    onChange={(e) => setAlertConfig((prev) => ({ ...prev, startFromEnd: e.target.checked }))}
                  />
                  <span className="slider" />
                </label>
              </div>
            </div>
          </div>
          <div className="inputs config-fields">
            <div className="input">
              <label>规则文件路径</label>
              <input
                placeholder="/etc/gwf/alert-rules.yaml"
                value={alertConfig.rulesFile}
                disabled={configDisabled}
                onChange={(e) => setAlertConfig((prev) => ({ ...prev, rulesFile: e.target.value }))}
              />
            </div>
            <div className="input">
              <label>日志路径</label>
              <input
                placeholder="/var/log/app/error.log,/var/log/app/worker.error.log"
                value={alertConfig.logPaths}
                disabled={configDisabled}
                onChange={(e) => setAlertConfig((prev) => ({ ...prev, logPaths: e.target.value }))}
              />
            </div>
            <div className="input">
              <label>轮询间隔</label>
              <input
                placeholder="2s / 5s / 10s"
                value={alertConfig.pollInterval}
                disabled={configDisabled}
                onChange={(e) => setAlertConfig((prev) => ({ ...prev, pollInterval: e.target.value }))}
              />
            </div>
          </div>
        </div>
        <div className="toolbar config-actions">
          <div className="toolbar-actions">
            <button className="btn" type="button" onClick={() => void handleConfigSave()} disabled={configDisabled}>
              {configSaving ? "保存中..." : "保存配置"}
            </button>
          </div>
        </div>
        {configMessage ? <div className="badge">{configMessage}</div> : null}
        {configError ? <div className="warn-text">{configError}</div> : null}
      </section>

      <section className="card decisions-card">
        <div className="section-header">
          <div>
            <h2>告警列表</h2>
            <p className="muted">展示最近决策与抑制原因</p>
          </div>
          <div className="section-meta">
            共 {decisions.length} 条{hasPagination ? ` · 第 ${decisionPage}/${totalDecisionPages} 页` : ""}
          </div>
        </div>
        {decisions.length === 0 ? (
          <div className="empty-state">暂无告警决策</div>
        ) : (
          <div className="table-wrap">
            <table className="decisions-table">
              <colgroup>
                <col className="col-time" />
                <col className="col-level" />
                <col className="col-rule" />
                <col className="col-message" />
                <col className="col-status" />
              </colgroup>
              <thead>
                <tr>
                  <th>时间</th>
                  <th>级别</th>
                  <th>规则</th>
                  <th>内容</th>
                  <th>状态</th>
                </tr>
              </thead>
              <tbody>
                {pagedDecisions.map((decision) => (
                  <tr key={decision.id}>
                    <td className="mono">{decision.time}</td>
                    <td>
                      <span className={`badge ${resolveLevelTone(decision.level)}`}>
                        {LEVEL_LABELS[decision.level]}
                      </span>
                    </td>
                    <td>{decision.rule}</td>
                    <td>
                      <div className="message-cell">
                        <span className="message-main">{decision.message}</span>
                        <span className="message-sub">{decision.file || "--"}</span>
                      </div>
                    </td>
                    <td>
                      <div className={`badge ${resolveStatusTone(decision.status)}`}>
                        {STATUS_LABELS[decision.status]}
                      </div>
                      {decision.reason ? <div className="status-reason">{decision.reason}</div> : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {hasPagination ? (
          <div className="decisions-footer">
            <div className="section-meta">
              第 {decisionPage} / {totalDecisionPages} 页
            </div>
            <div className="pagination">
              <button
                className="btn secondary"
                type="button"
                onClick={() => setDecisionPage((prev) => Math.max(1, prev - 1))}
                disabled={decisionPage <= 1}
              >
                上一页
              </button>
              <button
                className="btn secondary"
                type="button"
                onClick={() => setDecisionPage((prev) => Math.min(totalDecisionPages, prev + 1))}
                disabled={decisionPage >= totalDecisionPages}
              >
                下一页
              </button>
            </div>
          </div>
        ) : null}
      </section>

      <section className="summary-grid">
        <div className="card rules-card">
          <div className="section-header">
            <div>
              <h2>规则摘要</h2>
              <p className="muted">当前规则与分级统计</p>
            </div>
            <div className="section-meta">{formatTime(rules.lastLoaded)}</div>
          </div>
          <div className="summary-list">
            <div>
              <span>规则源</span>
              <strong>{rules.source}</strong>
            </div>
            <div>
              <span>规则数量</span>
              <strong>{rules.total}</strong>
            </div>
            <div>
              <span>默认抑制</span>
              <strong>{rules.defaultSuppress}</strong>
            </div>
            <div>
              <span>异常升级</span>
              <strong>{rules.escalation}</strong>
            </div>
          </div>
          <div className="levels-grid">
            <div className="level-pill">忽略 {rules.levels.ignore}</div>
            <div className="level-pill">业务 {rules.levels.business}</div>
            <div className="level-pill">系统 {rules.levels.system}</div>
            <div className="level-pill">致命 {rules.levels.fatal}</div>
          </div>
          {rules.error ? <div className="warn-text">规则加载异常: {rules.error}</div> : null}
        </div>

        <div className="card polling-card">
          <div className="section-header">
            <div>
              <h2>轮询摘要</h2>
              <p className="muted">日志文件与轮询节奏</p>
            </div>
            <div className="section-meta">{polling.interval}</div>
          </div>
          <div className="summary-list">
            <div>
              <span>最后轮询</span>
              <strong>{formatTime(polling.lastPoll)}</strong>
            </div>
            <div>
              <span>下一轮询</span>
              <strong>{formatTime(polling.nextPoll)}</strong>
            </div>
          </div>
          <div className="file-tags">
            {polling.logFiles.length === 0 ? (
              <span className="tag muted">未配置日志文件</span>
            ) : (
              polling.logFiles.map((file) => (
                <span className="tag" key={file}>
                  {file}
                </span>
              ))
            )}
          </div>
          {polling.error ? <div className="warn-text">轮询异常: {polling.error}</div> : null}
        </div>
      </section>

    </div>
  );
}

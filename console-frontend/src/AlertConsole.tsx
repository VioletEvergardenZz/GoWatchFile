import { useEffect, useMemo, useRef, useState } from "react";
import "./Alert.css";
import { alertConfigSnapshot, alertDashboard, alertRulesSnapshot } from "./mockData";
import type {
  AlertConfigResponse,
  AlertConfigSnapshot,
  AlertDashboard,
  AlertDecisionStatus,
  AlertLevel,
  AlertRulesResponse,
  AlertRulesSaveResponse,
  AlertRuleset,
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

type MatchCaseMode = "inherit" | "case" | "nocase";
type NotifyMode = "inherit" | "send" | "record";

type RuleDraft = {
  id: string;
  title: string;
  level: AlertLevel;
  keywordsText: string;
  excludesText: string;
  suppressWindow: string;
  matchCaseMode: MatchCaseMode;
  notifyMode: NotifyMode;
};

type RulesetDraft = {
  version: number;
  defaults: {
    suppressWindow: string;
    matchCase: boolean;
  };
  escalation: {
    enabled: boolean;
    level: AlertLevel;
    window: string;
    threshold: number;
    suppressWindow: string;
    ruleId: string;
    title: string;
    message: string;
  };
  rules: RuleDraft[];
};

const splitTokens = (value: string) =>
  value
    .split(/[\n,;，；\t]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0);

const joinTokens = (values: string[]) => values.join("\n");

const normalizeLevel = (value: string | undefined): AlertLevel => {
  if (value === "fatal" || value === "system" || value === "business" || value === "ignore") {
    return value;
  }
  return "business";
};

const createDefaultRuleset = (): RulesetDraft => ({
  version: 1,
  defaults: {
    suppressWindow: "5m",
    matchCase: false,
  },
  escalation: {
    enabled: true,
    level: "fatal",
    window: "5m",
    threshold: 20,
    suppressWindow: "5m",
    ruleId: "system_spike",
    title: "系统异常激增",
    message: "系统异常在5分钟内达到20次",
  },
  rules: [],
});

const createRuleDraft = (seed: Partial<RuleDraft> = {}): RuleDraft => ({
  id: seed.id ?? "",
  title: seed.title ?? "",
  level: seed.level ?? "business",
  keywordsText: seed.keywordsText ?? "",
  excludesText: seed.excludesText ?? "",
  suppressWindow: seed.suppressWindow ?? "",
  matchCaseMode: seed.matchCaseMode ?? "inherit",
  notifyMode: seed.notifyMode ?? "inherit",
});

const buildRulesetDraft = (ruleset: AlertRuleset | null | undefined): RulesetDraft => {
  const fallback = createDefaultRuleset();
  if (!ruleset) {
    return fallback;
  }
  const defaults = ruleset.defaults ?? {};
  const escalation = ruleset.escalation ?? {};
  const threshold = typeof escalation.threshold === "number" ? escalation.threshold : fallback.escalation.threshold;
  const enabled = escalation.enabled ?? threshold > 0;
  return {
    version: ruleset.version ?? fallback.version,
    defaults: {
      suppressWindow: defaults.suppress_window?.trim() || fallback.defaults.suppressWindow,
      matchCase: defaults.match_case ?? fallback.defaults.matchCase,
    },
    escalation: {
      enabled,
      level: normalizeLevel(escalation.level),
      window: escalation.window?.trim() || fallback.escalation.window,
      threshold,
      suppressWindow: escalation.suppress_window?.trim() || fallback.escalation.suppressWindow,
      ruleId: escalation.rule_id?.trim() || fallback.escalation.ruleId,
      title: escalation.title?.trim() || fallback.escalation.title,
      message: escalation.message?.trim() || fallback.escalation.message,
    },
    rules: (ruleset.rules ?? []).map((rule) => {
      const matchCaseMode: MatchCaseMode =
        rule.match_case === undefined ? "inherit" : rule.match_case ? "case" : "nocase";
      const notifyMode: NotifyMode = rule.notify === undefined ? "inherit" : rule.notify ? "send" : "record";
      return createRuleDraft({
        id: rule.id ?? "",
        title: rule.title ?? "",
        level: normalizeLevel(rule.level),
        keywordsText: joinTokens(rule.keywords ?? []),
        excludesText: joinTokens(rule.excludes ?? []),
        suppressWindow: rule.suppress_window ?? "",
        matchCaseMode,
        notifyMode,
      });
    }),
  };
};

const toApiRuleset = (draft: RulesetDraft): AlertRuleset => ({
  version: draft.version,
  defaults: {
    suppress_window: draft.defaults.suppressWindow.trim(),
    match_case: draft.defaults.matchCase,
  },
  escalation: {
    enabled: draft.escalation.enabled,
    level: draft.escalation.level,
    window: draft.escalation.window.trim(),
    threshold: Math.max(0, Math.floor(draft.escalation.threshold)),
    suppress_window: draft.escalation.suppressWindow.trim(),
    rule_id: draft.escalation.ruleId.trim(),
    title: draft.escalation.title.trim(),
    message: draft.escalation.message.trim(),
  },
  rules: draft.rules.map((rule) => {
    const keywords = splitTokens(rule.keywordsText);
    const excludes = splitTokens(rule.excludesText);
    const suppressWindow = rule.suppressWindow.trim();
    const matchCase = rule.matchCaseMode === "inherit" ? undefined : rule.matchCaseMode === "case";
    const notify =
      rule.level === "ignore" ? undefined : rule.notifyMode === "inherit" ? undefined : rule.notifyMode === "send";
    return {
      id: rule.id.trim(),
      title: rule.title.trim(),
      level: rule.level,
      keywords,
      excludes: excludes.length > 0 ? excludes : undefined,
      suppress_window: suppressWindow ? suppressWindow : undefined,
      match_case: matchCase,
      notify,
    };
  }),
});

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
  const [ruleset, setRuleset] = useState<RulesetDraft>(() =>
    USE_MOCK ? buildRulesetDraft(alertRulesSnapshot) : createDefaultRuleset()
  );
  const [rulesLoading, setRulesLoading] = useState(!USE_MOCK);
  const [rulesSaving, setRulesSaving] = useState(false);
  const [rulesMessage, setRulesMessage] = useState<string | null>(null);
  const [rulesError, setRulesError] = useState<string | null>(null);
  const [activePanel, setActivePanel] = useState<"rules" | "alerts" | "config">("rules");
  const [decisionPage, setDecisionPage] = useState(1);
  const [expandedGroups, setExpandedGroups] = useState<Record<AlertLevel, boolean>>(() => ({
    fatal: true,
    system: true,
    business: false,
    ignore: false,
  }));
  const [expandedRules, setExpandedRules] = useState<Record<number, boolean>>(() => ({}));
  const fetchingRef = useRef(false);
  const configFetchingRef = useRef(false);
  const rulesFetchingRef = useRef(false);
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

  const fetchRules = async () => {
    if (USE_MOCK || rulesFetchingRef.current || !aliveRef.current) return;
    rulesFetchingRef.current = true;
    setRulesLoading(true);
    setRulesError(null);
    try {
      const resp = await fetch(`${API_BASE}/api/alert-rules`, { cache: "no-store" });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(text || `接口异常 ${resp.status}`);
      }
      const payload = (await resp.json()) as AlertRulesResponse;
      if (!payload.ok || !payload.rules) {
        if (!aliveRef.current) return;
        setRulesError(payload.error ?? "获取规则失败");
        return;
      }
      if (!aliveRef.current) return;
      setRuleset(buildRulesetDraft(payload.rules));
    } catch (err) {
      if (!aliveRef.current) return;
      const msg = err instanceof Error ? err.message : "获取规则失败";
      setRulesError(msg);
    } finally {
      rulesFetchingRef.current = false;
      if (aliveRef.current) {
        setRulesLoading(false);
      }
    }
  };

  useEffect(() => {
    if (USE_MOCK) return;
    void fetchRules();
  }, []);

  const handleConfigSave = async () => {
    setConfigMessage(null);
    setConfigError(null);
    const logPaths = alertConfig.logPaths.trim();
    if (alertConfig.enabled) {
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

  const updateRule = (index: number, patch: Partial<RuleDraft>) => {
    setRuleset((prev) => {
      const nextRules = [...prev.rules];
      if (!nextRules[index]) return prev;
      nextRules[index] = { ...nextRules[index], ...patch };
      return { ...prev, rules: nextRules };
    });
  };

  const addRule = (seed?: Partial<RuleDraft>) => {
    setRuleset((prev) => ({ ...prev, rules: [...prev.rules, createRuleDraft(seed)] }));
  };

  const moveRule = (index: number, direction: -1 | 1) => {
    setRuleset((prev) => {
      const nextRules = [...prev.rules];
      const target = index + direction;
      if (target < 0 || target >= nextRules.length) return prev;
      const temp = nextRules[index];
      nextRules[index] = nextRules[target];
      nextRules[target] = temp;
      return { ...prev, rules: nextRules };
    });
    setExpandedRules((prev) => {
      const next = { ...prev };
      const target = index + direction;
      if (target < 0) return prev;
      if (next[index] || next[target]) {
        const current = !!next[index];
        const other = !!next[target];
        if (current) {
          delete next[index];
          next[target] = true;
        } else if (other) {
          delete next[target];
          next[index] = true;
        }
      }
      return next;
    });
  };

  const removeRule = (index: number) => {
    setRuleset((prev) => ({ ...prev, rules: prev.rules.filter((_, idx) => idx !== index) }));
    setExpandedRules((prev) => {
      const next: Record<number, boolean> = {};
      Object.keys(prev).forEach((key) => {
        const idx = Number(key);
        if (Number.isNaN(idx) || idx === index) return;
        next[idx > index ? idx - 1 : idx] = prev[idx];
      });
      return next;
    });
  };

  const duplicateRule = (index: number) => {
    setRuleset((prev) => {
      const nextRules = [...prev.rules];
      const base = nextRules[index];
      if (!base) return prev;
      const copy = createRuleDraft({
        ...base,
        id: "",
        title: base.title ? `${base.title} 副本` : "",
      });
      nextRules.splice(index + 1, 0, copy);
      return { ...prev, rules: nextRules };
    });
    setExpandedRules((prev) => {
      const next: Record<number, boolean> = {};
      Object.keys(prev).forEach((key) => {
        const idx = Number(key);
        if (Number.isNaN(idx)) return;
        next[idx > index ? idx + 1 : idx] = prev[idx];
      });
      next[index + 1] = true;
      return next;
    });
  };

  const handleRulesSave = async () => {
    setRulesMessage(null);
    setRulesError(null);
    const payload = toApiRuleset(ruleset);
    if (!payload.rules || payload.rules.length === 0) {
      setRulesError("请至少保留一条规则");
      return;
    }
    const invalidIndex = payload.rules.findIndex((rule) => !rule.keywords || rule.keywords.length === 0);
    if (invalidIndex >= 0) {
      setRulesError(`规则 ${invalidIndex + 1} 缺少关键词`);
      return;
    }
    if (ruleset.escalation.enabled) {
      if (!ruleset.escalation.window.trim() || ruleset.escalation.threshold <= 0) {
        setRulesError("已开启异常升级，请填写窗口与阈值");
        return;
      }
    }
    setRulesSaving(true);
    try {
      const resp = await fetch(`${API_BASE}/api/alert-rules`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ rules: payload }),
      });
      if (!resp.ok) {
        const text = await resp.text();
        throw new Error(text || `保存失败，状态码 ${resp.status}`);
      }
      const result = (await resp.json()) as AlertRulesSaveResponse;
      if (!result.ok || !result.rules) {
        setRulesError(result.error ?? "保存规则失败");
        return;
      }
      setRuleset(buildRulesetDraft(result.rules));
      setRulesMessage("规则已保存，下一轮询自动生效");
      await refreshDashboard();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "保存规则失败";
      setRulesError(msg);
    } finally {
      setRulesSaving(false);
    }
  };

  const overview = dashboard.overview;
  const stats = dashboard.stats;
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
  const rulesStatusLabel = rulesLoading
    ? "规则加载中"
    : ruleset.rules.length > 0
      ? `已加载 ${ruleset.rules.length} 条`
      : "暂无规则";
  const rulesDisabled = rulesLoading || rulesSaving;
  const rulesStats = useMemo(() => {
    const stats = {
      total: ruleset.rules.length,
      notify: 0,
      record: 0,
      ignore: 0,
      business: 0,
      system: 0,
      fatal: 0,
    };
    for (const rule of ruleset.rules) {
      stats[rule.level] = (stats[rule.level] ?? 0) + 1;
      if (rule.level === "ignore") continue;
      const notify =
        rule.notifyMode === "inherit" ? rule.level === "system" || rule.level === "fatal" : rule.notifyMode === "send";
      if (notify) {
        stats.notify += 1;
      } else {
        stats.record += 1;
      }
    }
    return stats;
  }, [ruleset.rules]);
  const levelOrder: AlertLevel[] = ["fatal", "system", "business", "ignore"];
  const groupedRules = useMemo(() => {
    const groups: Record<AlertLevel, Array<{ rule: RuleDraft; index: number }>> = {
      fatal: [],
      system: [],
      business: [],
      ignore: [],
    };
    ruleset.rules.forEach((rule, index) => {
      groups[rule.level].push({ rule, index });
    });
    return groups;
  }, [ruleset.rules]);
  const tabItems = useMemo(
    () => [
      {
        id: "rules" as const,
        label: "规则",
        meta: `${rulesStats.total} 条规则`,
      },
      {
        id: "alerts" as const,
        label: "告警",
        meta: `${decisions.length} 条决策`,
      },
      {
        id: "config" as const,
        label: "配置",
        meta: configStatusLabel,
      },
    ],
    [rulesStats.total, decisions.length, configStatusLabel]
  );

  const toggleGroup = (level: AlertLevel) => {
    setExpandedGroups((prev) => ({ ...prev, [level]: !prev[level] }));
  };

  const toggleRule = (index: number) => {
    setExpandedRules((prev) => ({ ...prev, [index]: !prev[index] }));
  };

  const addRuleInGroup = (level: AlertLevel) => {
    const nextIndex = ruleset.rules.length;
    addRule({ level });
    setExpandedGroups((prev) => ({ ...prev, [level]: true }));
    setExpandedRules((prev) => ({ ...prev, [nextIndex]: true }));
  };

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

      <nav className="alert-tabs" role="tablist" aria-label="告警控制台视图">
        {tabItems.map((item) => (
          <button
            key={item.id}
            type="button"
            className={`tab-button ${activePanel === item.id ? "active" : ""}`}
            onClick={() => setActivePanel(item.id)}
            role="tab"
            aria-selected={activePanel === item.id}
          >
            <span className="tab-label">{item.label}</span>
            <span className="tab-meta">{item.meta}</span>
          </button>
        ))}
      </nav>

      {activePanel === "alerts" ? (
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
      ) : null}

      {activePanel === "config" ? (
        <section className="card alert-config-card">
        <div className="section-header">
          <div>
            <h2>告警配置</h2>
            <p className="muted">运行时生效，保存到 config.runtime.yaml。</p>
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
      ) : null}

      {activePanel === "rules" ? (
        <section className="card alert-rules-card">
        <div className="section-header">
          <div>
            <h2>规则工作台</h2>
            <p className="muted">用可视化方式设置告警规则，系统/致命及时提醒，业务错误仅记录。</p>
          </div>
          <div className="section-meta">{rulesStatusLabel}</div>
        </div>
        <div className="rules-layout">
          <div className="rules-block rules-summary">
            <div className="block-head">
              <div>
                <div className="heading-with-hint">
                  <h3>规则概览</h3>
                  <span
                    className="hint-icon"
                    data-tip="用于快速查看各级别与通知策略的数量分布，帮助判断规则是否覆盖到位。"
                    aria-label="规则概览说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </div>
                <p className="muted small">规则总数 {rulesStats.total} 条</p>
              </div>
            </div>
            <div className="summary-row">
              <div className="summary-item level-fatal">
                <span>致命</span>
                <strong>{rulesStats.fatal}</strong>
              </div>
              <div className="summary-item level-system">
                <span>系统</span>
                <strong>{rulesStats.system}</strong>
              </div>
              <div className="summary-item level-business">
                <span>业务</span>
                <strong>{rulesStats.business}</strong>
              </div>
              <div className="summary-item level-ignore">
                <span>忽略</span>
                <strong>{rulesStats.ignore}</strong>
              </div>
            </div>
            <div className="summary-row">
              <div className="summary-item status-sent">
                <span>发送</span>
                <strong>{rulesStats.notify}</strong>
              </div>
              <div className="summary-item status-recorded">
                <span>仅记录</span>
                <strong>{rulesStats.record}</strong>
              </div>
            </div>
            <p className="muted small">业务错误可设置为仅记录，避免打扰。</p>
          </div>

          <div className="rules-block">
            <div className="block-head">
              <div>
                <div className="heading-with-hint">
                  <h3>默认策略</h3>
                  <span
                    className="hint-icon"
                    data-tip="未单独设置的规则将继承这里的抑制窗口与大小写策略。"
                    aria-label="默认策略说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </div>
                <p className="muted small">未单独配置的规则将继承这里的默认值</p>
              </div>
              <span className="badge ghost">全局</span>
            </div>
            <div className="inputs rules-grid">
              <div className="input">
                <label>
                  默认抑制窗口
                  <span
                    className="hint-icon"
                    data-tip="同一条规则在这个时间内只提醒一次，例如 5m/10m。"
                    aria-label="默认抑制窗口说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <input
                  placeholder="5m / 10m / 1h"
                  value={ruleset.defaults.suppressWindow}
                  disabled={rulesDisabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      defaults: { ...prev.defaults, suppressWindow: e.target.value },
                    }))
                  }
                />
              </div>
              <div className="input">
                <label>
                  默认大小写
                  <span
                    className="hint-icon"
                    data-tip="关闭时不区分大小写；开启后只匹配完全相同的大小写。"
                    aria-label="默认大小写说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <select
                  value={ruleset.defaults.matchCase ? "true" : "false"}
                  disabled={rulesDisabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      defaults: { ...prev.defaults, matchCase: e.target.value === "true" },
                    }))
                  }
                >
                  <option value="false">不区分大小写</option>
                  <option value="true">区分大小写</option>
                </select>
              </div>
            </div>
          </div>

          <div className="rules-block">
            <div className="block-head">
              <div>
                <div className="heading-with-hint">
                  <h3>异常升级</h3>
                  <span
                    className="hint-icon"
                    data-tip="当 system 级别在短时间内激增时自动升级为更高等级告警。"
                    aria-label="异常升级说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </div>
                <p className="muted small">system 告警短时间激增时触发升级</p>
              </div>
              <div className="switch-group">
                <span className="muted small">{ruleset.escalation.enabled ? "已启用" : "已关闭"}</span>
                <label className="switch mini">
                  <input
                    type="checkbox"
                    checked={ruleset.escalation.enabled}
                    disabled={rulesDisabled}
                    onChange={(e) =>
                      setRuleset((prev) => ({
                        ...prev,
                        escalation: { ...prev.escalation, enabled: e.target.checked },
                      }))
                    }
                  />
                  <span className="slider" />
                </label>
              </div>
            </div>
            <div className="inputs rules-grid">
              <div className="input">
                <label>
                  触发阈值
                  <span
                    className="hint-icon"
                    data-tip="在统计窗口内达到该次数即触发升级。"
                    aria-label="触发阈值说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <input
                  type="number"
                  min={1}
                  value={ruleset.escalation.threshold}
                  disabled={rulesDisabled || !ruleset.escalation.enabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      escalation: { ...prev.escalation, threshold: Number(e.target.value) || 0 },
                    }))
                  }
                />
              </div>
              <div className="input">
                <label>
                  统计窗口
                  <span
                    className="hint-icon"
                    data-tip="用于统计 system 级别告警的时间范围，如 5m/10m。"
                    aria-label="统计窗口说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <input
                  placeholder="5m / 10m"
                  value={ruleset.escalation.window}
                  disabled={rulesDisabled || !ruleset.escalation.enabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      escalation: { ...prev.escalation, window: e.target.value },
                    }))
                  }
                />
              </div>
              <div className="input">
                <label>
                  升级级别
                  <span
                    className="hint-icon"
                    data-tip="升级触发时发送的告警级别，通常为 system 或 fatal。"
                    aria-label="升级级别说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <select
                  value={ruleset.escalation.level}
                  disabled={rulesDisabled || !ruleset.escalation.enabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      escalation: { ...prev.escalation, level: e.target.value as AlertLevel },
                    }))
                  }
                >
                  <option value="fatal">致命</option>
                  <option value="system">系统</option>
                  <option value="business">业务</option>
                </select>
              </div>
              <div className="input">
                <label>
                  升级抑制窗口
                  <span
                    className="hint-icon"
                    data-tip="升级告警在该时间内只提醒一次，避免反复打扰。"
                    aria-label="升级抑制窗口说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <input
                  placeholder="5m / 10m"
                  value={ruleset.escalation.suppressWindow}
                  disabled={rulesDisabled || !ruleset.escalation.enabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      escalation: { ...prev.escalation, suppressWindow: e.target.value },
                    }))
                  }
                />
              </div>
            </div>
            <div className="inputs rules-grid">
              <div className="input">
                <label>
                  规则ID
                  <span
                    className="hint-icon"
                    data-tip="升级规则的唯一标识，便于日志与通知定位。"
                    aria-label="规则ID说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <input
                  placeholder="system_spike"
                  value={ruleset.escalation.ruleId}
                  disabled={rulesDisabled || !ruleset.escalation.enabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      escalation: { ...prev.escalation, ruleId: e.target.value },
                    }))
                  }
                />
              </div>
              <div className="input">
                <label>
                  升级标题
                  <span
                    className="hint-icon"
                    data-tip="升级告警展示的标题，可根据业务调整。"
                    aria-label="升级标题说明"
                    tabIndex={0}
                  >
                    i
                  </span>
                </label>
                <input
                  placeholder="系统异常激增"
                  value={ruleset.escalation.title}
                  disabled={rulesDisabled || !ruleset.escalation.enabled}
                  onChange={(e) =>
                    setRuleset((prev) => ({
                      ...prev,
                      escalation: { ...prev.escalation, title: e.target.value },
                    }))
                  }
                />
              </div>
            </div>
            <div className="input">
              <label>
                升级说明
                <span
                  className="hint-icon"
                  data-tip="升级告警的详细描述，例如触发条件说明。"
                  aria-label="升级说明"
                  tabIndex={0}
                >
                  i
                </span>
              </label>
              <textarea
                placeholder="系统异常在5分钟内达到20次"
                value={ruleset.escalation.message}
                disabled={rulesDisabled || !ruleset.escalation.enabled}
                onChange={(e) =>
                  setRuleset((prev) => ({
                    ...prev,
                    escalation: { ...prev.escalation, message: e.target.value },
                  }))
                }
              />
              </div>
          </div>

          <div className="rules-block rules-list-block">
            <div className="block-head">
              <div>
                <h3>规则列表</h3>
                <p className="muted small">按全局编号顺序匹配，命中即停止；可用上下移动调整顺序</p>
              </div>
              <div className="rule-toolbar" />
            </div>
            <div className="rule-list">
              {levelOrder.map((level) => {
                const items = groupedRules[level];
                const isOpen = expandedGroups[level];
                return (
                  <div className="rule-group" key={level}>
                    <div className={`rule-group-toggle ${isOpen ? "open" : ""}`}>
                      <button type="button" onClick={() => toggleGroup(level)}>
                        <span className={`badge ${resolveLevelTone(level)}`}>{LEVEL_LABELS[level]}</span>
                        <span className="group-meta">{items.length} 条</span>
                        <span className="group-toggle-text">{isOpen ? "收起" : "展开"}</span>
                      </button>
                      <button
                        className="btn secondary"
                        type="button"
                        onClick={() => addRuleInGroup(level)}
                        disabled={rulesDisabled}
                      >
                        新增{LEVEL_LABELS[level]}规则
                      </button>
                    </div>
                    {isOpen ? (
                      items.length === 0 ? (
                        <div className="empty-state small">暂无该级别规则</div>
                      ) : (
                        <div className="rule-group-body">
                          {items.map(({ rule, index }) => {
                            const isExpanded = !!expandedRules[index];
                            const keywords = splitTokens(rule.keywordsText);
                            const keywordPreview =
                              keywords.length === 0
                                ? "未填写关键词"
                                : `${keywords.slice(0, 2).join("、")}${keywords.length > 2 ? ` 等${keywords.length}条` : ""}`;
                            const notifyLabel =
                              rule.level === "ignore"
                                ? "忽略"
                                : rule.notifyMode === "inherit"
                                  ? "自动"
                                  : rule.notifyMode === "send"
                                    ? "发送"
                                    : "仅记录";
                            const suppressLabel =
                              rule.suppressWindow.trim() || `默认 ${ruleset.defaults.suppressWindow}`;
                            return (
                              <div className={`rule-card ${resolveLevelTone(rule.level)}`} key={`${rule.id}-${index}`}>
                                <div className="rule-head">
                                  <div className="rule-title">
                                    <span className={`badge ${resolveLevelTone(rule.level)}`}>{LEVEL_LABELS[rule.level]}</span>
                                    <span className="rule-index">#{index + 1}</span>
                                    <span className="rule-name">{rule.title || "未命名规则"}</span>
                                  </div>
                                  <div className="rule-actions">
                                    <button
                                      className="icon-btn"
                                      type="button"
                                      onClick={() => toggleRule(index)}
                                      disabled={rulesDisabled}
                                    >
                                      {isExpanded ? "收起" : "展开"}
                                    </button>
                                    <button
                                      className="icon-btn"
                                      type="button"
                                      onClick={() => moveRule(index, -1)}
                                      disabled={rulesDisabled || index === 0}
                                    >
                                      上移
                                    </button>
                                    <button
                                      className="icon-btn"
                                      type="button"
                                      onClick={() => moveRule(index, 1)}
                                      disabled={rulesDisabled || index === ruleset.rules.length - 1}
                                    >
                                      下移
                                    </button>
                                    <button
                                      className="icon-btn"
                                      type="button"
                                      onClick={() => duplicateRule(index)}
                                      disabled={rulesDisabled}
                                    >
                                      复制
                                    </button>
                                    <button
                                      className="icon-btn danger"
                                      type="button"
                                      onClick={() => removeRule(index)}
                                      disabled={rulesDisabled}
                                    >
                                      删除
                                    </button>
                                  </div>
                                </div>
                                {!isExpanded ? (
                                  <div className="rule-summary">
                                    <span>关键词：{keywordPreview}</span>
                                    <span>通知：{notifyLabel}</span>
                                    <span>抑制：{suppressLabel}</span>
                                  </div>
                                ) : null}
                                {isExpanded ? (
                                  <div className="rule-body">
                                    <div className="input">
                                      <label>
                                        规则名称
                                        <span
                                          className="hint-icon"
                                          data-tip="用于识别该规则的描述性名称。"
                                          aria-label="规则名称说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <input
                                        placeholder="例如：手机号校验失败"
                                        value={rule.title}
                                        disabled={rulesDisabled}
                                        onChange={(e) => updateRule(index, { title: e.target.value })}
                                      />
                                    </div>
                                    <div className="input">
                                      <label>
                                        规则级别
                                        <span
                                          className="hint-icon"
                                          data-tip="决定告警的重要程度与默认通知策略。"
                                          aria-label="规则级别说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <select
                                        value={rule.level}
                                        disabled={rulesDisabled}
                                        onChange={(e) => {
                                          const nextLevel = e.target.value as AlertLevel;
                                          updateRule(index, {
                                            level: nextLevel,
                                            notifyMode: nextLevel === "ignore" ? "inherit" : rule.notifyMode,
                                          });
                                        }}
                                      >
                                        <option value="ignore">忽略</option>
                                        <option value="business">业务</option>
                                        <option value="system">系统</option>
                                        <option value="fatal">致命</option>
                                      </select>
                                    </div>
                                    <div className="input">
                                      <label>
                                        通知策略
                                        <span
                                          className="hint-icon"
                                          data-tip="发送提醒或仅记录；自动会跟随级别默认策略。"
                                          aria-label="通知策略说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <select
                                        value={rule.notifyMode}
                                        disabled={rulesDisabled || rule.level === "ignore"}
                                        onChange={(e) => updateRule(index, { notifyMode: e.target.value as NotifyMode })}
                                      >
                                        <option value="inherit">自动（跟随级别）</option>
                                        <option value="send">发送提醒</option>
                                        <option value="record">仅记录</option>
                                      </select>
                                    </div>
                                    <div className="input">
                                      <label>
                                        规则 ID
                                        <span
                                          className="hint-icon"
                                          data-tip="留空会自动生成，推荐保持简短可读。"
                                          aria-label="规则ID说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <input
                                        placeholder="留空自动生成"
                                        value={rule.id}
                                        disabled={rulesDisabled}
                                        onChange={(e) => updateRule(index, { id: e.target.value })}
                                      />
                                    </div>
                                    <div className="input full">
                                      <label>
                                        关键词（任一命中触发）
                                        <span
                                          className="hint-icon"
                                          data-tip="任意关键词出现即触发，可多行填写。"
                                          aria-label="关键词说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <textarea
                                        placeholder="每行一个关键词，支持逗号分隔"
                                        value={rule.keywordsText}
                                        disabled={rulesDisabled}
                                        onChange={(e) => updateRule(index, { keywordsText: e.target.value })}
                                      />
                                      <span className="muted small">建议覆盖核心错误信息，如 timeout / 连接池耗尽。</span>
                                    </div>
                                    <div className="input full">
                                      <label>
                                        排除词（命中则忽略该规则）
                                        <span
                                          className="hint-icon"
                                          data-tip="出现排除词时不会触发该规则。"
                                          aria-label="排除词说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <textarea
                                        placeholder="例如：BusinessException"
                                        value={rule.excludesText}
                                        disabled={rulesDisabled}
                                        onChange={(e) => updateRule(index, { excludesText: e.target.value })}
                                      />
                                    </div>
                                    <div className="input">
                                      <label>
                                        抑制窗口
                                        <span
                                          className="hint-icon"
                                          data-tip="设置后该规则在窗口内只提醒一次。"
                                          aria-label="抑制窗口说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <input
                                        placeholder={`默认 ${ruleset.defaults.suppressWindow}`}
                                        value={rule.suppressWindow}
                                        disabled={rulesDisabled}
                                        onChange={(e) => updateRule(index, { suppressWindow: e.target.value })}
                                      />
                                    </div>
                                    <div className="input">
                                      <label>
                                        大小写
                                        <span
                                          className="hint-icon"
                                          data-tip="选择是否区分大小写，默认跟随全局。"
                                          aria-label="大小写说明"
                                          tabIndex={0}
                                        >
                                          i
                                        </span>
                                      </label>
                                      <select
                                        value={rule.matchCaseMode}
                                        disabled={rulesDisabled}
                                        onChange={(e) => updateRule(index, { matchCaseMode: e.target.value as MatchCaseMode })}
                                      >
                                        <option value="inherit">跟随默认</option>
                                        <option value="case">区分大小写</option>
                                        <option value="nocase">不区分大小写</option>
                                      </select>
                                    </div>
                                  </div>
                                ) : null}
                              </div>
                            );
                          })}
                        </div>
                      )
                    ) : null}
                  </div>
                );
              })}
            </div>
          </div>
        </div>
        <div className="toolbar rules-actions">
          <div className="toolbar-actions">
            <button className="btn" type="button" onClick={() => void handleRulesSave()} disabled={rulesDisabled}>
              {rulesSaving ? "保存中..." : "保存规则"}
            </button>
            <button className="btn secondary" type="button" onClick={() => void fetchRules()} disabled={rulesDisabled}>
              刷新规则
            </button>
          </div>
        </div>
        {rulesMessage ? <div className="badge">{rulesMessage}</div> : null}
        {rulesError ? <div className="warn-text">{rulesError}</div> : null}
        </section>
      ) : null}

      {activePanel === "alerts" ? (
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
                        {decision.analysis ? <span className="message-ai">AI：{decision.analysis}</span> : null}
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
      ) : null}

      {activePanel === "alerts" ? (
        <section className="summary-grid">
        <div className="card runtime-card">
          <div className="section-header">
            <div>
              <h2>告警运行态</h2>
              <p className="muted">当前开关与基础参数</p>
            </div>
            <div className="section-meta">{configStatusLabel}</div>
          </div>
          <div className="summary-list">
            <div>
              <span>告警开关</span>
              <strong>{alertConfig.enabled ? "启用" : "停用"}</strong>
            </div>
            <div>
              <span>告警抑制</span>
              <strong>{alertConfig.suppressEnabled ? "开启" : "关闭"}</strong>
            </div>
            <div>
              <span>轮询间隔</span>
              <strong>{alertConfig.pollInterval || "--"}</strong>
            </div>
            <div>
              <span>从末尾开始</span>
              <strong>{alertConfig.startFromEnd ? "是" : "否"}</strong>
            </div>
          </div>
          <div className="file-tags">
            {alertConfig.logPaths.trim() === "" ? (
              <span className="tag muted">未配置日志路径</span>
            ) : (
              alertConfig.logPaths
                .split(/[\n,;，；\t]+/)
                .map((item) => item.trim())
                .filter((item) => item.length > 0)
                .map((file) => (
                  <span className="tag" key={file}>
                    {file}
                  </span>
                ))
            )}
          </div>
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
      ) : null}

    </div>
  );
}

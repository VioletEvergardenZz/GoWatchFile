# 告警模式使用指南

> 适用：`go-watch-file` 的告警决策模块（日志轮询 → 规则匹配 → 决策记录/通知）。

## 1. 功能概览
- 轮询指定日志文件，按规则匹配日志行，输出告警决策。
- 支持规则级别、大小写、排除关键词、抑制窗口。
- system 级别可触发“异常升级”告警。
- 告警通知复用钉钉机器人与邮件配置（发送时机为“已发送”决策）。

## 2. 快速开始
1) **准备规则文件**
   - 参考 `go-watch-file/alert-rules.example.yaml`。
2) **配置告警相关项**
   - `alert_rules_file` 与 `alert_log_paths` 为必填项。
3) **启动服务**
   - 启动后日志会出现：`告警轮询已启动`。
4) **验证 API**
   - `GET /api/alerts` 返回告警概览、列表、统计、规则摘要与轮询摘要。

最小配置示例：
```yaml
alert_enabled: true
alert_suppress_enabled: true
alert_rules_file: "/etc/gwf/alert-rules.yaml"
alert_log_paths: "/var/log/app/error.log"
alert_poll_interval: "2s"
alert_start_from_end: true
```

## 3. 告警配置项详解

### 3.1 开启条件
以下任一条件满足时会启用告警模块：
- `alert_enabled: true`
- 或同时配置了 `alert_rules_file` 与 `alert_log_paths`

### 3.2 配置项说明
- `alert_enabled`：显式启用开关。
- `alert_suppress_enabled`：是否开启告警抑制（默认 `true`）。
- `alert_rules_file`：规则文件路径（YAML/JSON 均可）。
- `alert_log_paths`：日志文件路径列表，支持逗号/分号/空白/中文“，/；”分隔。
- `alert_poll_interval`：轮询间隔（默认 `2s`，支持 `2s`/`2秒`/`2`）。
- `alert_start_from_end`：是否从文件末尾开始追踪；默认 `true`。

说明：
- `alert_log_paths` 会做路径清理与去重。
- 规则文件必须存在；否则启动时报错。
- 告警相关配置不支持 `/api/config` 热更新，但可通过 `/api/alert-config` 运行时更新（可持久化到 `config.runtime.yaml`，重启后读取）。
- 规则文件内容支持热加载（修改后下次轮询自动生效）。

## 4. 规则文件结构

规则文件结构如下（字段均为可选/必填混合，示例为常用写法）：
```yaml
version: 1
defaults:
  suppress_window: 5m
  match_case: false

escalation:
  enabled: true
  level: fatal
  window: 5m
  threshold: 20
  suppress_window: 5m
  rule_id: system_spike
  title: 系统异常激增
  message: 系统异常在5分钟内达到20次

rules:
  - id: ignore_debug
    title: 低优先级忽略
    level: ignore
    keywords: [debug, trace]
    excludes: [important]
  - id: system_db
    title: 数据库连接池耗尽
    level: system
    keywords: ["Connection is not available", "连接池耗尽"]
```

### 4.1 defaults（默认值）
- `suppress_window`：默认抑制窗口，规则不写时使用。
- `match_case`：默认大小写策略（`true` 区分大小写，`false` 不区分）。

### 4.2 rules（规则列表）
单条规则字段：
- `id`/`title`：为空会自动生成/回填。
- `level`：`ignore|business|system|fatal`。
- `keywords`：关键字列表，**任一命中即匹配**（子串匹配）。
- `excludes`：排除关键字列表，**任一命中则本规则失效**。
- `suppress_window`：抑制窗口，支持 `5m/5分钟/300` 等写法。
- `match_case`：是否区分大小写（覆盖 defaults）。
- `notify`：是否发送通知；默认 `system/fatal=true`，其他为 `false`。

### 4.3 匹配行为（重要）
- **按顺序匹配**：命中即返回，不会继续匹配后续规则。
- `ignore` 规则命中后 **不会记录**，但会“吞掉”该行（后续规则不再匹配）。
- `excludes` 只影响当前规则；若排除命中，本行可能匹配后续规则。

## 5. 级别与决策状态

### 5.1 告警级别
- `ignore`：忽略，不记录、不通知。
- `business`：业务异常，默认只记录。
- `system`：系统异常，默认记录 + 通知。
- `fatal`：致命异常，默认记录 + 通知。

### 5.2 决策状态
- `sent`：已发送通知。
- `suppressed`：命中抑制窗口，已抑制。
- `recorded`：仅记录未通知（如 `notify: false`）。

## 6. 抑制与异常升级

### 6.1 抑制窗口
- 每条规则独立抑制；在窗口内重复匹配会被标记为 `suppressed`。
- 时间写法同轮询间隔：`2s/2秒/2`、`5m/5分钟/300`、`1h/1小时`。
- 可通过 `alert_suppress_enabled: false` 关闭抑制，所有匹配将直接发送通知。

### 6.2 异常升级（escalation）
- 统计 **system 级别**告警在 `window` 内的次数。
- 当次数 ≥ `threshold` 时，触发升级告警。
- `suppress_window` 用于升级抑制，未设置时默认等于 `window`。
- `enabled` 为空时，`threshold > 0` 自动启用。
- 升级触发会生成一条新的决策记录，文件字段为空。

## 7. 运行机制与注意事项
- **轮询读取**：按 `alert_poll_interval` 读取新增内容。
- **起始位置**：
  - `alert_start_from_end: true` → 只处理新写入日志。
  - `false` → 启动时从头扫描，可能产生大量历史告警。
- **概览窗口**：告警态势概览统计最近 24 小时的记录。
- **日志截断**：若文件变小（被截断/切割），读取游标会重置为 0。
- **行处理**：空行忽略；单行消息会截断到 240 字符。
- **记录上限**：告警列表只保留最近 200 条；统计为累计值。
- **规则热更新**：规则文件改动后自动重载；抑制状态与升级计数会重置。

## 8. API 与控制台
- `GET /api/alerts`：
  - `overview`：近 24 小时概览与风险等级，`window` 为窗口文案（如“最近24小时”）。
  - `decisions`：告警列表（最新在前）。
  - `stats`：累计统计（sent/suppressed/recorded）。
  - `rules`：规则摘要与加载状态。
  - `polling`：轮询摘要（最近轮询、下次轮询、错误信息）。
- `GET /api/alert-config`：读取告警配置快照。
- `POST /api/alert-config`：更新告警配置（如可写则持久化到 `config.runtime.yaml`，重启后读取）。
  - Persists to `config.runtime.yaml` when writable.

- 若告警未启用，API 返回 `error: "告警未启用"`。

## 9. 常见问题
- **为什么没有任何告警？**
  - 检查是否命中规则关键词，`alert_start_from_end` 是否忽略了历史日志。
- **为什么没有通知？**
  - 规则的 `notify` 可能为 `false`，或处于抑制窗口内。
  - 钉钉/邮件配置未填或无效。
- **规则生效顺序不对？**
  - 规则按顺序匹配；把“忽略规则”放前面，精确规则放后面。

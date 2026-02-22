# 控制面 Agent 在线状态判定与巡检手册

> 文档状态：兼容保留
> 更新时间：2026-02-19  
> 适用范围：`go-watch-file` 控制面（Agent 注册/心跳/分发）

## 1. 目的

本手册用于统一“Agent 是否在线”的判定口径，并提供可执行巡检步骤，避免出现：

- 控制台看起来是 `online`，但调度侧已按离线处理；
- 指标、接口、值班口径不一致，导致误判。

## 2. 在线判定口径（当前实现）

以下口径与后端代码一致：

- 心跳离线阈值：`45s`
  - 代码位置：`go-watch-file/internal/api/control_dispatch.go` 中 `defaultControlAgentOfflineAfter`
- 判定函数：`controlAgentIsActive(agent, now)`
  - 代码位置：`go-watch-file/internal/api/control_dispatch.go`
- `draining` 不计入在线数
  - 代码位置：`go-watch-file/internal/api/server.go`（控制面指标快照汇总）
- `status=offline` 不计入在线数
- `lastSeenAt` 为空时按在线处理（兼容首轮注册/恢复阶段）

判定逻辑可概括为：

1. `status=draining` -> 非在线  
2. `status=offline` -> 非在线  
3. `lastSeenAt` 为空 -> 在线  
4. `now - lastSeenAt <= 45s` -> 在线  
5. 其余 -> 心跳超时离线

## 3. 巡检入口

## 3.1 接口观测

- Agent 列表：`GET /api/control/agents`
- 指标对账：`GET /metrics`
  - `gwf_control_agents_total`
  - `gwf_control_agents_online`
  - `gwf_control_agent_heartbeat_lag_seconds`

## 3.2 一键巡检脚本（推荐）

脚本：`go-watch-file/scripts/ops/control-agent-check.ps1`

示例：

```powershell
cd go-watch-file
powershell -ExecutionPolicy Bypass -File scripts/ops/control-agent-check.ps1 `
  -BaseUrl http://localhost:8082 `
  -OfflineAfterSec 45 `
  -FailOnOffline `
  -MaxOfflineAgents 0 `
  -OutputFile ../reports/control-agent-check-result.json `
  -ReportFile ../docs/05-指标与评估/控制面Agent在线巡检报告-$(Get-Date -Format yyyy-MM-dd).md
```

说明：

- `-FailOnOffline` 开启门禁时：
  - 当离线数 `> MaxOfflineAgents` 返回退出码 `4`
- 输出：
  - JSON：结构化结果（用于自动化流程）
  - Markdown：值班/复盘报告（用于人工阅读）

## 4. 日常巡检流程（建议）

1. 执行 `control-agent-check.ps1`，查看 `summary.offlineTotal` 与 `maxLagSeconds`
2. 对账 `/metrics`：
   - 本地巡检汇总 vs `gwf_control_agents_total/online`
3. 若存在离线：
   - 优先查看 `offline_stale`（心跳超时）
   - 再查看 `offline_flag`（状态显式离线）
4. 对离线 Agent 做分组确认：
   - 网络/进程异常
   - 版本不兼容
   - 发布窗口人工摘流（`draining`）
5. 记录处置动作到阶段报告与审计日志

## 5. 异常处置建议

## 5.1 `offline_stale` 持续增长

- 排查 Agent 到控制面网络连通性
- 检查 Agent 心跳逻辑是否被阻塞
- 观察 `gwf_control_agent_heartbeat_lag_seconds` 是否持续上升

## 5.2 `offline_flag` 异常增长

- 检查是否误操作将 Agent 标记为离线
- 检查恢复动作是否执行（心跳或拉取应可拉回在线）

## 5.3 `draining` 数量异常

- 核对发布计划与摘流窗口
- 避免长时间遗留在 `draining` 导致可用容量不足

## 6. 与控制台展示口径说明

- 控制台 Agent 列中的 `status` 为 Agent 当前状态字段；
- 指标中的 `gwf_control_agents_online` 为“按心跳窗口计算后的在线数”；
- 当两者不一致时，以心跳窗口判定（调度口径）为准，并按本手册流程排查。

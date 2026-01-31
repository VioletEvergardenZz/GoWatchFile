# 系统资源管理控制台接口说明

本文说明资源管理控制台所依赖的后端接口、字段含义与采样策略。

---

## 1. 接口

**GET `/api/system`**

Query 参数：
- `mode=lite`：仅返回概览/指标/分区，不包含进程列表。
- `limit=200`：限制返回进程数量；`0` 表示不限制；不传时默认 200。

返回结构：
```json
{
  "systemOverview": {},
  "systemGauges": [],
  "systemVolumes": [],
  "systemProcesses": []
}
```

启用要求：
- 需在控制台开启 `systemResourceEnabled`（`/api/config` 可更新），否则接口返回 403。

---

## 2. 字段说明

### 2.1 systemOverview
- `host`：主机名。
- `os`：系统版本（平台 + 版本）。
- `kernel`：内核版本。
- `uptime`：运行时长（中文格式）。
- `load`：负载均值（1/5/15 分钟）。
- `ip`：主机 IP（取首个非环回 IPv4）。
- `lastUpdated`：采样时间（`HH:MM:SS`）。
- `processes`：进程总数。
- `connections`：活动连接总数（优先排除 LISTEN）。
- `connectionsBreakdown`：连接状态摘要。
- `cpuTemp`：CPU 温度（无法读取时为 `--`）。
- `topProcess`：CPU 占用最高的进程名称。

### 2.2 systemGauges
`id` 固定为 `cpu` / `memory` / `disk`，其余字段为前端展示文案：
- `usedPct`：使用率百分比。
- `usedLabel` / `totalLabel`：已用/总量文案。
- `subLabel` / `trend`：辅助说明与趋势文案。

### 2.3 systemVolumes
- `mount`：挂载点。
- `usedPct`：分区使用率。
- `used` / `total`：已用/总量。

### 2.4 systemProcesses
与前端 `SystemProcess` 对齐，包含 PID、CPU/内存占比、I/O 速率、监听端口、工作目录与环境变量等。

---

## 3. 采样策略与注意事项
- **CPU/IO 速率**：由两次采样差值计算，首次请求可能显示 `--`。
- **端口/环境变量**：受系统权限限制，权限不足时可能为空。
- **NetIn/NetOut**：目前为占位值 `--`，如需精确网络速率需额外采样实现。
- **性能控制**：建议前端使用 `mode=lite` + 轮询、按需加载进程列表。

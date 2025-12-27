# File Watch 控制台（前端）

基于 `docs/prototype/index.html` 的视觉原型重构为 React + TypeScript 单页应用，便于后续接入后端 API。

## 快速开始
```bash
cd console-frontend
npm install   # 已在本机安装过可跳过
npm run dev   # 本地开发
npm run build # 产出 dist/
```

默认通过 Vite 将 `/api` 代理到 `http://localhost:8080`，若后端地址不同可设置 `VITE_API_BASE`。

## 结构说明
- `src/App.tsx`：主界面与交互逻辑（目录树、文件表格、Tail、监控图表）。
- `src/mockData.ts`：当前使用的静态数据，接入后端时可替换为 fetch/API。
- `src/types.ts`：数据结构定义，确保前后端字段对齐。
- `src/App.css` / `src/index.css`：主题与布局样式，延续原型的深色质感与分区。

## 接口接入提示
1) 将 `mockData.ts` 中的数组替换为接口请求，并在 `App.tsx` 中用 `useEffect` 加载。
2) 目录树、文件列表、失败队列等模块使用路径作为唯一键，后端返回时保持唯一性。
3) 图表数据形如 `[{ label, uploads, failures, queue }]`，可直接映射到 `chartPoints`。

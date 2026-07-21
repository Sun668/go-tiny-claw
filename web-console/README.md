# Tiny Claw Web Console

这是一个支持多 Session 并行执行的 Agent 控制台。每个 Session 使用一条独立 WebSocket 连接，对应服务端的一个 `ChannelSession` 和 `Runtime`。

## 启动

先启动 Go Server：

```bash
cd /Users/smsun/Documents/github/go-tiny-claw
export ZHIPU_API_KEY="你的 API Key"
go run ./cmd/claw_server
```

再启动前端：

```bash
cd web-console
npm install
npm run dev
```

浏览器打开 `http://localhost:5173`。

开发服务器会把 `/ws` 代理到 `ws://127.0.0.1:8081/ws`。生产环境可以通过 `VITE_WS_URL` 指定完整 WebSocket 地址。

## 功能

- 新建、切换、关闭多个 Session
- 每个 Session 独立上下文并行执行
- 实时显示流式文本、工具调用和工具结果
- 支持中断当前任务
- 支持允许一次、会话允许和拒绝审批

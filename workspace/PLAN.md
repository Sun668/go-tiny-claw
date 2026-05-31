# 极简 Go Web Server 项目规划

## 目标
搭建一个极简的 Go 语言 Web Server 项目，结构清晰、开箱即用。

## 架构设计
- 使用 Go 标准库 `net/http`，零依赖
- 项目结构遵循 Go 社区惯例
- 支持 JSON 响应、路由分组、优雅关停

## 项目结构
```
.
├── cmd/
│   └── server/
│       └── main.go          # 入口，启动服务器
├── internal/
│   ├── handler/
│   │   └── handler.go       # HTTP 处理函数
│   └── middleware/
│       └── middleware.go     # 中间件（日志、恢复）
├── go.mod
├── PLAN.md
└── TODO.md
```

## 技术选型
- 语言：Go 1.21+
- HTTP 框架：标准库 `net/http`（Go 1.22+ 增强路由）
- 中间件：手写链式中间件
- 优雅关停：`os/signal` + `http.Server.Shutdown`

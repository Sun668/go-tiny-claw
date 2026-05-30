# 极简 Go Web Server 项目计划

## 目标
搭建一个极简的 Go 语言 Web Server 项目，结构清晰、可运行、可扩展。

## 架构设计
- 使用 Go 标准库 `net/http` 构建，零外部依赖
- 采用经典的 Go 项目布局
- 支持路由分发、JSON 响应、中间件（日志）机制

## 项目结构
```
.
├── cmd/
│   └── server/
│       └── main.go          # 入口，启动 HTTP Server
├── internal/
│   ├── handler/
│   │   └── handler.go       # 路由处理函数
│   └── middleware/
│       └── logging.go       # 日志中间件
├── go.mod                   # Go 模块定义
├── PLAN.md
└── TODO.md
```

## 技术选型
- Go 1.21+
- 标准库 `net/http`（不引入第三方框架）
- 路由：基于 `http.ServeMux` 的简单路由
- 中间件：函数链式包装模式

## API 端点
| 方法 | 路径       | 说明         |
|------|-----------|-------------|
| GET  | /         | 欢迎页面      |
| GET  | /health   | 健康检查      |
| GET  | /api/info | 返回 JSON 信息 |

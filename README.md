# wx_channel_insight

微信视频号运营数据分析工具。通过独立运行的 `wx_channel` 数据面采集数据，在 Insight 中管理账号与关键词监控、查询视频指标和评论洞察。

当前页面包括概览、账号监控、关键词监控、视频分析、运营 Agent 和设置。AI 用于解释已有数据，未配置 AI 时核心数据功能仍可使用。历史选题、脚本、制作与发布流程已移除；旧页面路由别名、工作流 API 和 Windows 计划任务入口不再提供兼容。

## 本地快速启动

需要 Go 1.25+、Node.js 22+ 和 npm。以下命令在项目根目录的 PowerShell 中执行：

```powershell
Copy-Item config.example.json insight.local.json
Set-Location apps/web
npm ci
npm run build
Set-Location ../..
go run ./apps/api
```

打开启动日志中的本地地址，默认 <http://127.0.0.1:18081/>。模板使用 SQLite，数据库位于 `var/insight.db`，无需安装 PostgreSQL 或 Redis。模板启用本机 operator，只能用于本机访问。

另开一个终端，在同一项目目录启动监控调度：

```powershell
go run ./apps/scheduler
```

真实采集需要另外启动 `wx_channel`，默认连接 `http://127.0.0.1:2026`，视频号客户端需处于就绪状态。Insight 不包含该程序或其登录会话。

## 配置与存储

API、jobs 和 scheduler 共用 JSON 配置。默认读取当前工作目录的 `insight.local.json`，也可指定：

```powershell
$env:INSIGHT_CONFIG_FILE = 'C:\opt\insight\insight.local.json'
```

优先级为环境变量 > JSON > 内置默认值。JSON 中的相对文件路径以配置文件所在目录解析；环境变量中的相对路径以进程工作目录解析。公开字段及环境变量映射见 [配置实现](internal/runtime/config_file.go)。配置字段拼写错误、重复字段或类型错误会阻止启动。

| 方式 | `storage.driver` | 用途 |
| --- | --- | --- |
| SQLite | `sqlite` | 本机持久存储，默认公开模板 |
| PostgreSQL | `postgres` | 服务端/团队部署，参考 `config.postgres.example.json` |
| JSON 文件 | `file` | 按模块保存 JSON，数据位于 `storage.data_dir` |
| 内存 | `memory` | 临时演示和测试，重启不保留 |

`stores` 可覆盖单个读模型的驱动与路径；通常只需设置统一的 `storage.driver`。切换驱动不会自动搬迁数据。读模型只读写当前 JSON 结构，不再自动识别或改写旧格式文件。存储迁移工具见 [开发文档](docs/README.md#存储与迁移)。

PostgreSQL 模板中的 DSN 和桥接 token 需要自行配置。运行 `go run ./apps/jobs bootstrap-schema` 初始化迁移后再启动 API。面向浏览器的团队部署使用服务端可信代理，参考 [Docker 部署](deploy/stack/DEPLOYMENT.md)。共享 operator 部署没有独立用户登录体系；对外服务还需部署方配置访问控制。

AI 密钥通过 `OPENAI_API_KEY` 或本地配置提供；在设置中持久保存 AI 服务配置时需设置 `INSIGHT_AI_CONFIG_ENCRYPTION_KEY`（64 位十六进制）。本地配置、密钥和运行数据库不进入 Git 或分发包。

## 使用运行包

- [Windows 运行包](deploy/windows/README.md)：解压后启动 API 与 scheduler，首次启动生成本地配置。
- [Docker 源码包部署](deploy/stack/DEPLOYMENT.md)：联网构建镜像，运行 PostgreSQL、Redis、API、Web 和 scheduler。
- [Docker 离线包部署](deploy/stack/DEPLOYMENT-OFFLINE.md)：使用已导出的 Linux 镜像，无需现场编译。

构建命令和产物内容见 [打包开发指南](docs/packaging.md)。

## 项目结构

| 路径 | 职责 |
| --- | --- |
| `apps/api`、`apps/jobs`、`apps/scheduler` | HTTP 服务、一次性任务、周期调度 |
| `apps/web` | Vue 3 + Vite 前端 |
| `internal`、`migrations` | 业务、存储实现与 PostgreSQL 迁移 |
| `docs` | 开发文档、设计规格与历史计划 |
| `scripts` | 运行、构建及诊断脚本 |
| `deploy` | 部署模板及运行包使用说明 |
| `tests/scripts` | 脚本回归测试 |
| `bin`、`release`、`var` | 本地生成物，均不纳入版本控制 |

开发和验证从 [docs/README.md](docs/README.md) 开始；脚本用途见 [scripts/README.md](scripts/README.md)。

## 开源协议

[Apache License 2.0](LICENSE)。使用者需自行确认其数据访问、账号授权及第三方服务的使用条件。

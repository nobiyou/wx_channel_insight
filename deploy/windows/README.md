# Windows 运行包

解压到可写的新目录，双击 `scripts\windows\Start-Insight-Preview.cmd`。需要 Windows 和 PowerShell 5.1/7；运行机器无需 Go 或 Node.js。

首次启动从 `config.example.json` 复制 `insight.local.json`，默认仅监听 `127.0.0.1:18081`，使用 `data/insight.db` SQLite 库。随后启动 API 和 scheduler，打开本地页面。再次启动保留已修改的配置。停止请运行 `scripts\windows\Stop-Insight-Preview.cmd`。

## 配置

编辑 `insight.local.json` 后重启。JSON 中的相对路径以配置文件目录解析。环境变量可覆盖 JSON；避免残留旧 `INSIGHT_*` 环境变量导致存储或端口与文件不一致。

`wx_channel.base_url` 默认为 `http://127.0.0.1:2026`，真实采集需独立运行 `wx_channel` 及就绪的视频号客户端。评论导出目录默认 `data/comment_data`，可改为实际导出目录。

手动启动及只读预览：

```powershell
.\scripts\windows\start-insight-preview.ps1 -PackageRoot . -NoBrowser
.\scripts\windows\start-insight-preview.ps1 -PackageRoot . -EmitOnly
```

临时端口覆盖用 `-APIAddr 127.0.0.1:18082`，启动器也会相应调整 scheduler 地址。直接修改 JSON 时，请同步修改 `server.addr` 和 `scheduler.api_base_url`。占用的端口会拒绝启动，不会接管已有 API。

## PostgreSQL 与 jobs

可从 `config.postgres.example.json` 另建本地配置，填写 DSN 和桥接 token，通过 `-ConfigFile` 选择。团队/远程访问需可信代理；此目录包默认本机 operator，不提供公网认证代理。

在包根目录执行 jobs，和 API 使用同一配置：

```powershell
$env:INSIGHT_CONFIG_FILE = (Join-Path (Get-Location) 'insight.local.json')
.\bin\insight-jobs.exe status
# PostgreSQL 首次使用需执行：.\bin\insight-jobs.exe bootstrap-schema
```

AI 密钥可通过本地配置或 `OPENAI_API_KEY` 提供。在设置页保存 AI 服务配置前，设置 `INSIGHT_AI_CONFIG_ENCRYPTION_KEY`（64 位十六进制）并妥善保存该密钥。

## 升级与排查

日志在 `data/logs/`，进程记录在 `data/preview-runtime.json`。启动失败先查看 `api-error.log` / `scheduler-error.log`。停止器仅终止能校验包内可执行路径和启动时间的进程。

升级前停止旧 API/scheduler 并备份配置和数据，解压新包到新目录后迁移或引用原数据路径。不要覆盖正在使用的目录或只复制运行中的 SQLite 主文件。`manifest.json` 记录构建来源；`LICENSE` 为 Apache-2.0。

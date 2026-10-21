# 脚本入口

`scripts/` 存放可执行的运行、构建和诊断脚本，开发测试位于 `tests/scripts/windows`。PowerShell 脚本的参数以各文件头部为准；在源码仓库运行时从项目根目录调用。

## 常用入口

| 场景 | `windows/` 中的脚本 | 说明 |
| --- | --- | --- |
| Windows 包启动/停止 | `Start-Insight-Preview.cmd` / `Stop-Insight-Preview.cmd` | 仅用于已构建的 Windows 目录包 |
| Windows 配置启动 | `start-insight-preview.ps1` | `-ConfigFile` 选择配置，`-APIAddr` 临时改端口，`-NoBrowser` 不开浏览器 |
| 预览 Windows 启动计划 | `start-insight-preview.ps1 -EmitOnly` | 不创建配置或数据，不输出密钥 |
| Docker 源码栈 | `start-insight-docker-stack.ps1` / `stop-insight-docker-stack.ps1` | 需先准备 `deploy/stack/.env`，构建加 `-Build` |
| Docker 验证 | `verify-insight-docker-stack.ps1` | 服务健康、可信桥接和可选业务验证；先看参数再执行 |
| 离线包导入/停止 | `import-and-start-insight-offline.ps1` / `stop-insight-offline.ps1` | 打包后命名为 `import-and-start-insight.ps1` / `stop-insight.ps1` |
| 构建 Windows 包 | `build-insight-preview.ps1` | Go + npm 构建到 `release/` |
| 构建 Docker 源码包 | `build-insight-docker-package.ps1` | 构建输入白名单，不带本地运行数据 |
| 构建 Docker 离线包 | `build-insight-docker-offline-package.ps1` | 显式选择已验证的 API/Web 镜像 |
| 单独构建 jobs | `build-insight-jobs.ps1` | 输出到 `bin/`，编译失败保留原 exe |

`package-common.ps1` 是打包脚本内部共用函数，不作为独立运行入口。完整打包约束见 [docs/packaging.md](../docs/packaging.md)，包内使用说明在 [deploy](../deploy/stack/README.md)。

## 高级开发与诊断

源码开发与排障使用以下入口：

| 脚本组 | 用途 |
| --- | --- |
| `start-insight-trusted-web-dev.ps1` | 本地可信代理 Web 开发 |
| `diagnose-insight-runtime.ps1` | 只读检查当前 HTTP 服务、端口和可用依赖工具 |
| `restore-wx-channel-page-client.ps1` | 数据面客户端诊断/恢复工具，执行前查看参数及计划 |

开发与诊断入口不分发到运行包。可信 Web 开发代理使用显式环境变量配置 API 地址与身份，不再自动读取历史运行记录。Docker 验证通过 jobs 命令将可信证据保存到 `var/trusted-runtime-evidence.json`，运行状态只读取此标准文件。支持 `-EmitOnly` 的入口可先预览，但仍需阅读具体脚本确认其副作用。

## 持续监控调度

持续监控使用 `apps/scheduler`；Windows 运行包由启动器管理 scheduler，Docker 使用 Compose 中的 scheduler 服务。运行状态 API 提供当前调度健康信息。源码不提供旧 Windows 计划任务脚本，也不生成注册命令。

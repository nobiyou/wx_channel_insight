# Docker 源码构建部署

需要 Docker Engine/Desktop（Linux engine）、Compose v2、可用的镜像/依赖网络，以及独立运行的 `wx_channel`。Windows 使用 PowerShell 5.1/7；Linux/macOS 可直接调用 Compose。

以下命令均在仓库或解压后的源码包根目录执行。源码 ZIP 中 Web 使用 `source-built` target；Git 仓库的默认 target 为 `prebuilt`，需先在 `apps/web` 执行 `npm ci` 和 `npm run build`。

## 配置

```powershell
Copy-Item deploy/stack/.env.example deploy/stack/.env
```

编辑 `deploy/stack/.env`，设置以下值后才能启动：

| 配置 | 要求 |
| --- | --- |
| `INSIGHT_POSTGRES_PASSWORD` | 自行生成数据库密码；DSN 中特殊字符需按 URL 规则编码，建议使用随机十六进制值 |
| `INSIGHT_TRUSTED_BRIDGE_TOKEN` | 自行生成的高强度随机 token，Web 代理和 scheduler 与 API 共用 |
| `INSIGHT_AI_CONFIG_ENCRYPTION_KEY` | 64 位十六进制密钥，备份时一并保存 |
| `INSIGHT_WX_CHANNEL_BASE_URL` | 容器能访问的数据面地址，默认 `http://host.docker.internal:2026` |

模板中的 `replace-with-*` 是占位符。不要提交 `.env`。评论目录使用下面脚本参数传入，或将绝对路径设置为 `INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR`。`OPENAI_API_KEY` 可留空。

Compose 使用环境变量配置 API/jobs/scheduler，不加载根目录的本地 JSON。Nginx 在服务端添加可信桥接头；浏览器直接打开 Web 根地址即可。该配置使用共享 operator 身份，外网使用前需增加访问控制和 TLS。

## 启动与检查

Windows：

```powershell
New-Item -ItemType Directory -Force data/comment_data
.\scripts\windows\start-insight-docker-stack.ps1 -RepoRoot . -Build -WxChannelCommentDataDir .\data\comment_data
```

脚本默认使用 DaoCloud 基础镜像镜像源，可通过 `INSIGHT_DOCKER_MIRROR` 选择自己的 registry 前缀。

Linux/macOS（已准备上述 `.env`）：

```bash
mkdir -p data/comment_data
export INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR="$PWD/data/comment_data"
docker compose --env-file deploy/stack/.env -f deploy/stack/docker-compose.yml config --quiet
docker compose --env-file deploy/stack/.env -f deploy/stack/docker-compose.yml up -d --build
```

栈包含 PostgreSQL、Redis、一次性迁移、API、Web 和 scheduler。等待健康检查后打开 <http://127.0.0.1:5176/>；API 健康检查为 <http://127.0.0.1:18081/api/healthz>。默认还占用本机 `15432`、`16379`。启动前确认没有其他实例占用这些端口。

```powershell
docker compose --env-file deploy/stack/.env -f deploy/stack/docker-compose.yml ps
docker compose --env-file deploy/stack/.env -f deploy/stack/docker-compose.yml logs --tail 100 api scheduler
docker compose --env-file deploy/stack/.env -f deploy/stack/docker-compose.yml run --rm jobs status
```

真实采集要求 wx_channel 客户端就绪；Insight API 健康不代表采集链路可用。修改后的 `.env` 由下次 Compose 启动读取。

## 停止、升级和备份

```powershell
.\scripts\windows\stop-insight-docker-stack.ps1 -RepoRoot .
```

停止保留命名卷。PostgreSQL 业务数据和 API `var` 位于命名卷，评论文件来自只读宿主挂载。升级前备份数据库、运行状态、`.env` 和加密密钥，再在隔离环境验证迁移；不要以删除数据卷作为常规重启或排障方式。

离线场景改用 [离线运行包](DEPLOYMENT-OFFLINE.md)。

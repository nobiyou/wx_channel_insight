# Docker 离线运行包

本说明针对包含 `images/insight-runtime-images.tar` 的离线 ZIP。包根目录包含 Compose、`.env.example`、许可证、manifest 和运行脚本。无需 Go/Node 或现场构建；Docker engine 及 Compose v2 需事先安装，目标机器需与 manifest 中的 Linux 镜像架构匹配。

`wx_channel` 独立运行，默认地址 `http://host.docker.internal:2026`。真实采集还要求视频号客户端就绪。

## 配置后启动

进入解压目录，先复制并编辑配置：

```powershell
Copy-Item .env.example .env
```

必须将 `INSIGHT_POSTGRES_PASSWORD`、`INSIGHT_TRUSTED_BRIDGE_TOKEN`、`INSIGHT_AI_CONFIG_ENCRYPTION_KEY` 的占位符替换为自己的值；加密密钥需为 64 位十六进制。数据库密码建议使用随机十六进制以避免 DSN 编码问题。AI API key 可留空。Compose 使用 `.env`，不加载附带的开发 JSON 示例。

Windows（Docker Desktop Linux engine）：

```powershell
.\scripts\windows\import-and-start-insight.ps1 -RepoRoot .
```

脚本检查配置、导入镜像，然后启动服务。后续已导入时可加 `-SkipLoad`。评论目录默认 `data/comment_data`，可通过 `-CommentDataDir` 指向实际导出目录。

Linux/macOS：

```bash
cp .env.example .env
# 编辑 .env 并填好上述三个值
mkdir -p data/comment_data
export INSIGHT_WX_CHANNEL_COMMENT_DATA_DIR="$PWD/data/comment_data"
docker compose --env-file .env -f docker-compose.yml config --quiet
docker load --input images/insight-runtime-images.tar
docker compose --env-file .env -f docker-compose.yml up -d --pull never
```

等待服务健康后打开 <http://127.0.0.1:5176/>，API 健康地址为 <http://127.0.0.1:18081/api/healthz>。另外占用 `15432`、`16379`。Web 使用共享 operator 的可信服务端代理，外部访问需部署方增加访问控制和 TLS。

## 检查和停止

```powershell
docker compose --env-file .env -f docker-compose.yml ps
docker compose --env-file .env -f docker-compose.yml logs --tail 100 api scheduler
.\scripts\windows\stop-insight.ps1 -RepoRoot .
```

Linux/macOS 可用 `docker compose --env-file .env -f docker-compose.yml down` 停止，保留命名卷。不要把删除卷作为常规重启。升级前备份 PostgreSQL 数据、API 运行状态、`.env` 和加密密钥。

manifest 的 `images` 记录实际镜像引用、ID、平台和可用的 OCI revision。`packagingSource` 只标识打包代码，不证明镜像由该提交构建；来源未知的镜像需要分发前单独验收。

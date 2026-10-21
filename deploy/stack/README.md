# Docker 部署入口

- [源码构建部署](DEPLOYMENT.md)：仓库或源码 ZIP，联网构建 API/Web。
- [离线镜像部署](DEPLOYMENT-OFFLINE.md)：包含镜像 tar 的离线 ZIP，无需现场编译。
- [开发打包说明](../../docs/packaging.md)：如何产生上述包。

两种方式都需要先复制 `.env.example` 并设置数据库密码、可信桥接 token 和 AI 配置加密密钥。`wx_channel` 单独运行。开发依赖 Compose 位于 `deploy/local`，不是分发安装入口。

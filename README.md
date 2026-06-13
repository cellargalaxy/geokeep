# geokeep

低配版个人 GPS 数据保管服务（dawarich 子集）。

- 接收 [OwnTracks](https://owntracks.org/) 与 [Overland](https://github.com/aaronpk/Overland-iOS) HTTP 上报
- SQLite 单文件持久化；备份/恢复 = 复制 / 上传 `.db` 文件
- 双向兼容 [dawarich](https://dawarich.app) 数据（v1 `data.json` / v2 archive）
- OpenStreetMap + Leaflet 地图页（时间窗、设备过滤、抽样、回放）
- 纯 Go 编译（`CGO_ENABLED=0`），无外部依赖，单二进制 / Docker 部署
- 支持子路径反代（`domain.com/xxx/`）
- 首启 Web 向导初始化，无需进容器执行命令
- API Key 可在线轮换

需求与技术方案：见 `vibe/2606p0/` 目录。

## 快速开始

### 本地

```sh
export GEOKEEP_SECRET=$(head -c32 /dev/urandom | base64)
make build
./geokeep serve
# 浏览器访问 http://localhost:8080/ → 完成初始化向导
```

### Docker

```sh
docker run -d --name geokeep \
  -e GEOKEEP_SECRET="$(head -c32 /dev/urandom | base64)" \
  -v $PWD/data:/data \
  -p 8080:8080 \
  ghcr.io/<owner>/geokeep:main
```

镜像由 GitHub Actions（`.github/workflows/docker.yml`）在任意分支 push 时自动构建并推送至 GHCR。

## 子路径反代

```nginx
location /xxx/ {
    proxy_pass http://127.0.0.1:8080/xxx/;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Prefix /xxx;
}
```

```sh
GEOKEEP_BASE_PATH=/xxx ./geokeep serve
```

## 上报配置

OwnTracks 客户端 → Settings → Mode = HTTP：
- URL: `https://your.host/api/v1/owntracks/points?api_key=<KEY>`

Overland iOS → Settings → Receiver Endpoint：
- URL: `https://your.host/api/v1/overland/batches`
- Access Token: `<KEY>`

## CLI 子命令

```sh
geokeep serve                       # 默认
geokeep rotate-key --email a@b.c    # 离线轮换 api_key
geokeep restore --from /path/x.db   # 离线恢复
```

## 配置

| 环境变量 | 默认 | 说明 |
|---|---|---|
| `GEOKEEP_LISTEN` | `:8080` | HTTP 监听 |
| `GEOKEEP_DATA_DIR` | `./data` | 数据目录（含 `geokeep.db / imports/ / exports/ / backups/`） |
| `GEOKEEP_SECRET` | — | 必填，Session HMAC 密钥（≥ 16 字节，建议 32） |
| `GEOKEEP_BASE_PATH` | 空 | 子路径反代前缀，例如 `/xxx`；不带尾斜杠 |
| `GEOKEEP_MAX_UPLOAD_MB` | `5` | 上传字节上限 |
| `GEOKEEP_OSM_TILE_URL` | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` | OSM 瓦片模板 |
| `GEOKEEP_DEV` | `false` | 开发模式：允许 HTTP Cookie / 放开 CSRF |

## 备份 / 恢复

- 设置页 → 立即下载备份（走 `SQLite VACUUM INTO`，热备）
- 设置页 → 上传 `.db` 恢复 → 重启服务生效（启动期自动消费 `.pending_restore` 标记）
- 离线：`geokeep restore --from path.db`

## 越权防护

- 所有业务查询 SQL 强制 `WHERE user_id = ?`；repo 层入口校验
- Session 走 HMAC-SHA256 签名 Cookie；`HttpOnly + SameSite=Lax + Path=BasePath`
- 上报通道与 Web 通道严格分离（前者 api_key，后者 session）
- CSRF：非 GET 请求强校验 `Sec-Fetch-Site=same-origin` 或 `Origin`
- 登录失败 5 次后账号锁定 15 分钟
- 密码 bcrypt cost=12，最小长度 10

## 开发

```sh
make test          # 全套单测
make vet
make docker        # 本地构建镜像
```

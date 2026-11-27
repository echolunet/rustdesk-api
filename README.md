# RustDesk API Server

[![CI](https://github.com/echolunet/rustdesk-api/actions/workflows/ci.yml/badge.svg)](https://github.com/echolunet/rustdesk-api/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)
![RustDesk](https://img.shields.io/badge/RustDesk-Official%20Server-2563EB)

基于 **Go (Gin + GORM)** 自研的开源 RustDesk 后端 API 与现代化 Web 管理控制台，原生解耦编排**官方 `rustdesk/rustdesk-server:latest`**（`hbbs` + `hbbr`），兼具轻量化、账户与安全管控、2FA/邮箱验证、客户端默认策略（帧率/画质/权限）下发以及官方服务端无痛升级能力。

---

## 目录

- [架构概览](#架构概览)
- [核心特性](#核心特性)
- [快速部署](#快速部署)
- [Docker Compose 编排详解](#docker-compose-编排详解)
- [客户端配置与扫码导入](#客户端配置与扫码导入)
- [客户端默认策略下发（帧率 / 画质 / 权限）](#客户端默认策略下发帧率--画质--权限)
- [安全功能配置（2FA / 邮箱登录验证）](#安全功能配置2fa--邮箱登录验证)
- [更新官方服务端与 API](#更新官方服务端与-api)
- [切换外部数据库（MySQL / PostgreSQL）](#切换外部数据库mysql--postgresql)
- [HTTPS / WSS 与 Web 客户端托管](#https--wss-与-web-客户端托管)
- [常见问题与测试](#常见问题与测试)

---

## 架构概览

```text
  RustDesk 桌面端 / 移动端 / Web 客户端
          │                       │
          │ HTTP REST / WebAdmin  │ TCP/UDP 21115-21117 & WS 21118-21119
          ▼                       ▼
┌─────────────────────┐   ┌─────────────────────────────────┐
│  rustdesk-api (Go)  │   │  官方 rustdesk/rustdesk-server  │
│  - /api/* 客户端API │   │  - hbbs (ID / 信令 / WS 21118)  │
│  - /_admin/ 管理后台│   │  - hbbr (中继 / WS 21119)       │
│  - /webclient/ 托管 │   └────────────────┬────────────────┘
│  - /ws/id & /ws/relay│                   │ 持久化 ./data/server
└─────────┬───────────┘                    │ (自动生成 id_ed25519.pub)
          │ 只读挂载公钥                    │
          └────────────────────────────────┘
```

---

## 核心特性

- **Docker 一键部署 & 多架构预构建镜像**
  - GitHub Actions 自动构建 `linux/amd64` / `linux/arm64` 镜像并发布至 `ghcr.io/echolunet/rustdesk-api:latest`，默认 `docker-compose.yml` 无需本地安装 Go 编译器或拉取源码即可直接启动。
  - 提供交互式一键部署脚本 `scripts/install.sh`：自动识别服务器公网 IP、配置默认帧率（30 / 60 / 120 FPS）、生成 `.env` 并输出后台地址、初始密码和官方公钥。
- **官方服务端解耦 & 一键更新**
  - `hbbs`、`hbbr` 直接采用官方镜像 `rustdesk/rustdesk-server:latest` 并默认开启 `-k _` 公钥强校验，密钥持久化在 `./data/server`。
  - 支持 `./scripts/update.sh` 手动更新或启用 Watchtower Profile 定时自动拉取官方最新版，升级过程不丢失任何密钥或数据。
  - 管理后台内置**官方版本检测中心**，实时展示 `rustdesk/rustdesk` 客户端与 `rustdesk/rustdesk-server` 最新 Release 及下载入口。
- **客户端默认策略下发（帧率 FPS / 画质 / 权限）**
  - 支持在 `.env`（如 `CLIENT_DEFAULT_FPS=60`）与管理后台 **「设置 → 客户端策略」** 集中配置默认帧率（`5–120 FPS`）、图像质量、首选编解码器（VP9 / VP8 / H.264 / H.265 / AV1）、硬件编解码与会话权限开关，并通过 `/api/heartbeat` 增量下发至客户端。
- **完整兼容 RustDesk 客户端协议**
  - **认证与会话**：`/api/login`、`/api/logout`、`/api/currentUser`（JWT 持久化会话 + 内存短期缓存，撤销立即生效）。
  - **地址簿与标签**：完整支持 Legacy v1（`/api/ab`、`/api/ab/get`）与 Modern v2 个人/共享地址簿（`/api/ab/personal`、`/api/ab/shared/profiles`、`/api/ab/peers`、`/api/ab/tags/:guid`、`/api/ab/peer/*`、`/api/ab/tag/*`、`/api/ab/settings`），双向同步。
  - **设备与权限隔离**：`/api/heartbeat`、`/api/sysinfo`（内置 `peer_id + uuid` 归属校验，拒绝跨 UUID 覆盖设备信息）、`/api/peers`、`/api/devices`、`/api/users`、`/api/device-group/accessible`。
  - **审计上报**：`/api/audit/conn`、`/api/audit/file`、`/api/audit/alarm`。
- **2FA 两步验证 & SMTP 邮箱登录校验**
  - **TOTP 动态验证器**：支持 Google / Microsoft Authenticator / 1Password 等扫码绑定，内置 6 个一次性备用恢复码。
  - **SMTP 邮箱验证**：支持 `STARTTLS` / `SSL` / 无加密发信与测试邮件；支持 **关闭全局 / 仅新设备首次验证（自动记录信任设备） / 每次登录验证** 及单用户强制开关。
- **内置离线 Web 管理控制台（`/_admin/`） & SSO / LDAP**
  - 零外部 CDN 依赖（纯原生 HTML/CSS/JS 预压缩嵌入单二进制，支持 ETag/Gzip 协商、浅色/深色双主题与移动端抽屉布局）。
  - 支持通用 **OIDC**、**GitHub**、**Google** 与 **LDAP / Active Directory**（含在线测试连接）。
  - 内置 `/ws/id` → `hbbs:21118` 与 `/ws/relay` → `hbbr:21119` WebSocket 代理，附带生产级 `nginx/rustdesk.conf` HTTPS/WSS 配置及官方 Flutter Web 客户端同步脚本。

---

## 快速部署

### 方式一：一条命令一键部署（推荐）

在任意已安装 Docker 与 Docker Compose v2 的 Linux VPS 上执行：

```bash
curl -fsSL https://raw.githubusercontent.com/echolunet/rustdesk-api/main/scripts/install.sh | bash
```

脚本将自动探测公网 IP、交互式选择默认远程帧率（`60 FPS` / `120 FPS` / `30 FPS` / 自定义），拉取官方 `hbbs`/`hbbr` 与预构建 API 镜像启动全套服务，并输出后台地址、初始密码和官方 Public Key。若尚未开放 GHCR 公开访问或镜像未构建完成，脚本也会在检测到 `git` 时自动回退为源码本地构建。

> 支持非交互自动化传参：
> ```bash
> SERVER_HOST=your.domain.com DEFAULT_FPS=120 NON_INTERACTIVE=1 \
>   curl -fsSL https://raw.githubusercontent.com/echolunet/rustdesk-api/main/scripts/install.sh | bash
> ```

### 国内 VPS 加速部署与阿里云 ACR 双推

1. **一键脚本国内加速模式（`CN_MIRROR=1`）**

   通过 `gh-proxy.com` 拉取安装文件/源码，自动切换南大 `ghcr.nju.edu.cn` 镜像站及 `goproxy.cn`：
   ```bash
   CN_MIRROR=1 SERVER_HOST=你的服务器IP DEFAULT_FPS=60 NON_INTERACTIVE=1 \
     curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/echolunet/rustdesk-api/main/scripts/install.sh | bash
   ```
   若已开启下方阿里云 ACR 自动同步，也可直接指定拉取你自己的私有/公有 ACR 镜像：
   ```bash
   CN_MIRROR=1 SERVER_HOST=你的服务器IP API_IMAGE=registry.cn-hangzhou.aliyuncs.com/<你的命名空间>/rustdesk-api:latest \
     curl -fsSL https://gh-proxy.com/https://raw.githubusercontent.com/echolunet/rustdesk-api/main/scripts/install.sh | bash
   ```

2. **启用 GitHub Actions 自动双推阿里云个人版镜像仓库（ACR）**

   [.github/workflows/docker-publish.yml](.github/workflows/docker-publish.yml) 支持单次多架构构建后同时推送 GHCR 与阿里云 ACR。前往 GitHub 仓库 **Settings → Secrets and variables → Actions** 添加以下 4 个 Secret（未配置时自动跳过、仅推送 GHCR）：
   - `ACR_REGISTRY`：例如 `registry.cn-hangzhou.aliyuncs.com`
   - `ACR_NAMESPACE`：你的阿里云 ACR 命名空间名称
   - `ACR_USERNAME`：阿里云 ACR 访问账号（建议新建固定访问凭证/RAM 账号）
   - `ACR_PASSWORD`：对应镜像仓库固定密码

### 方式二：仅 Compose 直接拉取预构建镜像

无需克隆仓库或编译源码，仅下载编排与配置模板即可启动：

```bash
mkdir -p rustdesk-api && cd rustdesk-api
curl -fsSLO https://raw.githubusercontent.com/echolunet/rustdesk-api/main/docker-compose.yml
curl -fsSL https://raw.githubusercontent.com/echolunet/rustdesk-api/main/.env.example -o .env
```

编辑 `.env` 修改公网地址与默认帧率（其余 `CLIENT_*` 参数可按需调整）：

```env
RUSTDESK_ID_SERVER=your.domain.com:21116
RUSTDESK_RELAY_SERVER=your.domain.com:21117
RUSTDESK_API_SERVER=http://your.domain.com:21114
CLIENT_DEFAULT_FPS=60
```

拉取并启动：

```bash
docker compose pull && docker compose up -d
```

> **提示**：仓库首次推送触发 `Docker Publish` workflow 后，请在 GitHub **Packages** 将 `rustdesk-api` 设为 **Public**，此后任意服务器均可免登录直接 `docker compose pull`。

### 方式三：从源码本地构建

如需自行修改 Go 代码或自定义构建，可克隆仓库并叠加 [docker-compose.build.yml](docker-compose.build.yml)：

```bash
git clone https://github.com/echolunet/rustdesk-api.git
cd rustdesk-api
cp .env.example .env
# 编辑 .env 填入公网地址与默认参数（国内服务器可设置 GOPROXY=https://goproxy.cn,direct）
chmod +x scripts/*.sh
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

### 获取初始密码与官方公钥

```bash
# 查看首次启动自动生成的管理员账号与随机密码
docker logs rustdesk-api

# 查看官方 hbbs 生成的公钥（也可直接在后台首页查看/复制）
cat ./data/server/id_ed25519.pub
```

浏览器访问 `http://<你的服务器IP>:21114`（自动跳转至 `/_admin/`）登录管理后台并及时修改初始密码。

---

## Docker Compose 编排详解

[docker-compose.yml](docker-compose.yml) 使用桥接网络 `rustdesk-net`、统一 `json-file` 日志轮转（单文件上限 `10m`、最多保留 `3` 份）以及按需启用的 Compose Profiles。

### 1. 服务清单与启用方式

| Service | 容器名 | 镜像来源 | 启动条件 | 职责与特性 |
| :--- | :--- | :--- | :--- | :--- |
| `hbbs` | `rustdesk-hbbs` | `${RUSTDESK_SERVER_IMAGE:-rustdesk/rustdesk-server:latest}` | 默认启动 | 官方 RustDesk ID 注册与信令服务器，默认携带 `-k _` 公钥强校验，带 `watchtower.enable=true` |
| `hbbr` | `rustdesk-hbbr` | `${RUSTDESK_SERVER_IMAGE:-rustdesk/rustdesk-server:latest}` | 默认启动 | 官方 RustDesk 中继转发服务器，带 `watchtower.enable=true` |
| `rustdesk-api` | `rustdesk-api` | `${API_IMAGE:-ghcr.io/echolunet/rustdesk-api:latest}` | 默认启动 | 自研 API 后端、`/_admin/` 控制台、`/webclient/` 托管及 `/ws/*` 代理；内置 `/health` 健康检查（`watchtower.enable=false`） |
| `nginx` | `rustdesk-nginx` | `nginx:alpine` | `--profile nginx` | HTTPS/WSS 反向代理入口（TLS 终止、`80 -> 443` 跳转及 `/ws/id`、`/ws/relay` 转发） |
| `watchtower` | `rustdesk-watchtower` | `containrrr/watchtower:latest` | `--profile auto-update` | 按 `WATCHTOWER_INTERVAL` 周期检查并自动更新官方 `hbbs` 与 `hbbr` |

### 2. 端口放行与防火墙矩阵

| 端口 | 协议 | 所属服务 | 用途 |
| :--- | :--- | :--- | :--- |
| `21114` | TCP | `rustdesk-api` | 客户端 REST API、`/_admin/` 管理后台、`/webclient/`（可通过 `.env` 的 `API_PORT` 调整） |
| `21115` | TCP | `hbbs` | 官方 NAT 类型检测 |
| `21116` | TCP & UDP | `hbbs` | ID 注册、UDP 心跳保活与 TCP 打洞（**必须同时放行 TCP 与 UDP**） |
| `21117` | TCP | `hbbr` | 中继（Relay）流量转发 |
| `21118` | TCP | `hbbs` | `hbbs` WebSocket 信令端口（使用 Nginx HTTPS 反代时可仅内网访问） |
| `21119` | TCP | `hbbr` | `hbbr` WebSocket 中继端口（使用 Nginx HTTPS 反代时可仅内网访问） |
| `80` / `443` | TCP | `nginx` | 仅启用 `--profile nginx` 时需要放行，用于 HTTPS / WSS 访问 |

### 3. 目录与持久化卷映射

| 宿主机路径 | 容器挂载路径 | 读写权限 | 用途说明 |
| :--- | :--- | :--- | :--- |
| `./data/server` | `/root`（`hbbs`/`hbbr`） / `/data/server`（API） | 官方读写 / API 只读 | 持久化 `id_ed25519` / `id_ed25519.pub` 密钥对与 `hbbs` 状态，API 通过 `:ro` 读取公钥生成配置 |
| `./data/api` | `/app/data` | 读写 | 持久化 SQLite 数据库（`rustdesk-api.db` 及 WAL 文件）与自动生成的 `.jwt_secret` |
| `./data/webclient` | `/app/webclient` | 读写 | 存放通过 `scripts/sync-webclient.sh` 同步的官方 Flutter Web 客户端静态文件 |
| `./nginx/rustdesk.conf` | `/etc/nginx/conf.d/default.conf` | 只读 | `nginx` Profile 反向代理配置模板 |
| `./nginx/ssl` | `/etc/nginx/ssl` | 只读 | `nginx` Profile 所需的 `fullchain.pem` 与 `privkey.pem` SSL 证书 |

### 4. 关键环境变量分组（`.env`）

- **镜像与基础地址**：`API_IMAGE`、`RUSTDESK_SERVER_IMAGE`、`HBBS_EXTRA_ARGS`、`HBBR_EXTRA_ARGS`、`RUSTDESK_ID_SERVER`、`RUSTDESK_RELAY_SERVER`、`RUSTDESK_API_SERVER`、`API_PORT`、`TZ`
- **数据库与会话安全**：`DB_TYPE`（`sqlite` / `mysql` / `postgres`）、`DB_DSN`、`JWT_SECRET`、`JWT_EXPIRE_HOURS`、`ADMIN_INIT_USER`、`ADMIN_INIT_PASSWORD`
- **设备隔离与地址簿限制**：`ALLOW_ALL_DEVICES_FOR_USERS`、`MAX_PEER_ONE_AB`
- **客户端默认策略（首次建库写入，支持后台动态覆盖）**：`CLIENT_STRATEGY_ENABLED`、`CLIENT_DEFAULT_IMAGE_QUALITY`、`CLIENT_DEFAULT_FPS`、`CLIENT_DEFAULT_QUALITY`、`CLIENT_DEFAULT_CODEC`、`CLIENT_ENABLE_*`、`CLIENT_DIRECT_SERVER`、`CLIENT_VERIFICATION_METHOD`、`CLIENT_APPROVE_MODE`
- **源码构建与自动更新**：`GOPROXY`（叠加 `docker-compose.build.yml` 时生效）、`WATCHTOWER_INTERVAL`

### 5. Compose Profiles 与常用运维命令

```bash
# 仅启动核心服务（hbbs + hbbr + rustdesk-api）
docker compose up -d

# 启用内置 Nginx HTTPS 反代（需先准备好 ./nginx/ssl 证书）
docker compose --profile nginx up -d

# 启用 Watchtower 自动追踪官方 hbbs/hbbr 更新（可与 nginx 同时开启）
docker compose --profile nginx --profile auto-update up -d

# 使用本地源码覆盖构建并启动
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build

# 查看容器状态、健康状态与实时日志
docker compose ps
docker compose logs -f rustdesk-api hbbs hbbr
```

---

## 客户端配置与扫码导入

登录管理后台 **「概览」**，支持三种方式配置 RustDesk 客户端：

1. **手机端扫码**：打开 RustDesk App「网络设置」→ 点击右上角扫码图标，直接扫描后台生成的配置二维码。
2. **桌面端一键导入**：复制后台生成的「一键配置导入字符串」，或通过 CLI 静默部署：
   ```bash
   rustdesk --config "<配置导入字符串>"
   ```
3. **手动填写**：按后台展示的 ID 服务器、中继服务器、API 服务器与 Public Key 填入客户端「网络设置」。

---

## 客户端默认策略下发（帧率 / 画质 / 权限）

首次启动时会从 `.env` 读取默认客户端策略写入数据库，之后随时可在 Web 控制台 **「设置 → 客户端策略」** 实时修改并保存，系统会自动递增策略版本时间戳，在线客户端在下次 `/api/heartbeat` 心跳（通常 15 秒内）自动拉取生效：

| 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `CLIENT_STRATEGY_ENABLED` | `true` | 是否在心跳中启用默认策略下发 |
| `CLIENT_DEFAULT_IMAGE_QUALITY` | `custom` | 默认画质模式（设为 `custom` 时自定义帧率才生效） |
| `CLIENT_DEFAULT_FPS` | `60` | 默认传输帧率上限（范围 `5–120`） |
| `CLIENT_DEFAULT_QUALITY` | `75` | 自定义画质百分比（范围 `10–100`） |
| `CLIENT_DEFAULT_CODEC` | `auto` | 首选编解码器（`auto` / `vp9` / `vp8` / `h264` / `h265` / `av1`） |
| `CLIENT_ENABLE_HWCODEC` / `AUDIO` / `CLIPBOARD` / `FILE_TRANSFER` / `KEYBOARD` / `TUNNEL` / `REMOTE_RESTART` / `RECORD_SESSION` | 见 `.env.example` | 对应客户端硬解、音频、剪贴板、文件传输等默认权限开关 |
| `CLIENT_DIRECT_SERVER` | `false` | 是否默认开启直连服务器 |

---

## 安全功能配置（2FA / 邮箱登录验证）

1. **开启 TOTP 两步验证**：在后台左侧菜单进入 **「安全」** → 点击 **「立即开启 2FA」** → 使用验证器 App 扫描二维码并输入 6 位动态码，同时保存生成的 6 个一次性备用恢复码。
2. **开启 SMTP 邮箱验证**：在同一页面填写 SMTP 服务器（支持 `STARTTLS` / `SSL` / 无加密），先发送测试邮件验证连通性，再选择全局验证策略（**关闭** / **仅新设备首次登录验证** / **每次登录验证**）或前往 **「用户」** 为单个用户单独开启「强制邮箱验证」。
3. **管理员应急重置**：若用户更换或丢失验证器手机，管理员可在 **「用户」** 中点击该用户的 **「重置 2FA」**。

---

## 更新官方服务端与 API

### 更新逻辑与数据安全保障

- **完全解耦、独立升级**：自研 `rustdesk-api`（账户、设备、地址簿、2FA、策略、Web 管理台）与官方 `rustdesk-server`（`hbbs` 信令 + `hbbr` 中继）彼此解耦，可**仅更新官方服务端**、**仅更新 API**，或**一起更新**。
- **持久化隔离（升级不丢数据、不换公钥）**：
  - `./data/server`（或非 Docker 时 `hbbs`/`hbbr` 的工作目录）：保存 `id_ed25519` / `id_ed25519.pub` 密钥对及 `db_v2.sqlite3`。只要保留该目录，升级 `hbbs`/`hbbr` 后所有客户端**无需重新配置公钥**。
  - `./data/api`（`DATA_DIR`）：保存 SQLite 数据库 `rustdesk-api.db`（或外部 MySQL/PostgreSQL）及 `.jwt_secret`。新版 `rustdesk-api` 启动时通过 GORM `AutoMigrate` 自动向后兼容迁移表结构，用户、设备、地址簿、2FA、客户端策略与登录态全部保留。
- **前端自动防缓存**：Web 管理台（`/_admin/`）内嵌于 `rustdesk-api` 二进制并自动注入资源指纹 `?v=<hash>`，升级 API 后直接刷新浏览器即可生效。
- **目录提醒**：若使用 `scripts/install.sh` 一键部署且触发了源码回退构建，实际部署目录为 `/opt/rustdesk-api/src`（预构建镜像模式为 `/opt/rustdesk-api`）；执行更新前请先 `cd` 到包含 `.env` 和 `./data/` 的实际目录。

### 一、Docker 预构建镜像模式更新（GHCR / 阿里云 ACR）

#### 1. 使用内置脚本一键更新

```bash
# Linux / macOS
./scripts/update.sh            # 仅拉取并更新官方 rustdesk/rustdesk-server (hbbs & hbbr)
./scripts/update.sh --pull-api # 拉取最新预构建 rustdesk-api 镜像 + 更新官方 hbbs & hbbr
./scripts/update.sh --all      # Git 仓库场景先 git pull --ff-only，再拉取最新 API 与官方服务端镜像
```

```powershell
# Windows PowerShell
.\scripts\update.ps1           # 仅更新官方 hbbs & hbbr
.\scripts\update.ps1 -PullApi  # 拉取最新预构建 rustdesk-api 镜像并更新官方服务端
```

#### 2. 原生 `docker compose` 命令（不依赖脚本）

```bash
cd /opt/rustdesk-api   # 切换到包含 docker-compose.yml 和 .env 的目录

# 仅更新官方 RustDesk Server（hbbs & hbbr）
docker compose pull hbbs hbbr
docker compose up -d --no-deps hbbs hbbr

# 仅更新 rustdesk-api
docker compose pull rustdesk-api
docker compose up -d --no-deps rustdesk-api

# 全部一起更新并清理悬空旧镜像
docker compose pull
docker compose up -d --remove-orphans
docker image prune -f
```

> 若启用了内置 Nginx 反代，请将 `docker compose up -d` 替换为 `docker compose --profile nginx up -d`。国内 VPS 在 `.env` 中配置好 `API_IMAGE`（如阿里云 ACR 或 `ghcr.nju.edu.cn`）及 `RUSTDESK_SERVER_IMAGE` 后，拉取命令会自动走加速源。

### 二、Docker 源码本地构建模式更新（`docker-compose.build.yml`）

适用于通过 `git clone` 部署，或一键脚本自动回退到 `/opt/rustdesk-api/src` 本地构建的场景。

#### 1. 使用内置脚本

```bash
# Linux / macOS：拉取最新源码 + 重新编译 rustdesk-api + 拉取最新官方 hbbs & hbbr
./scripts/update.sh --git-pull --rebuild-api
```

```powershell
# Windows PowerShell
git pull --ff-only
.\scripts\update.ps1 -RebuildApi
```

#### 2. 原生 `git` + `docker compose` 命令

```bash
cd /opt/rustdesk-api/src   # 或你克隆仓库的源码目录

# 仅更新代码并重新构建 rustdesk-api
git pull --ff-only origin main
docker compose -f docker-compose.yml -f docker-compose.build.yml build --pull rustdesk-api
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --no-deps rustdesk-api

# 同时更新官方 hbbs/hbbr 与源码重建 rustdesk-api
git pull --ff-only origin main
docker compose -f docker-compose.yml -f docker-compose.build.yml pull hbbs hbbr
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build --remove-orphans
docker image prune -f
```

### 三、Docker 定时自动更新官方 `hbbs` & `hbbr`（Watchtower）

```bash
docker compose --profile auto-update up -d
```

编排通过容器标签做了隔离：仅带有 `com.centurylinklabs.watchtower.enable=true` 的官方 `hbbs`/`hbbr` 会按 `WATCHTOWER_INTERVAL`（默认 `86400` 秒 / 24 小时）自动追踪最新官方镜像；`rustdesk-api` 默认标记为 `false`，避免业务 API 无人值守变更。

### 四、原始 / 非 Docker 部署更新（宿主机二进制 / `systemd` / `pm2`）

#### 1. 更新 `rustdesk-api`（Go 源码重新编译替换二进制）

更新逻辑：`git pull` 拉取最新代码 → 编译静态单二进制（内嵌最新 Web 控制台）→ 备份旧二进制 → 停止服务并替换可执行文件 → 重启服务（原有 `DATA_DIR` 下的数据库与 `.jwt_secret` 自动保留并迁移）。

```bash
cd /opt/rustdesk-api-src   # 你的 Go 源码目录（需 Go 1.24+）
git pull --ff-only origin main

# 编译新版单二进制（海外服务器可省略 GOPROXY）
CGO_ENABLED=0 GOPROXY=https://goproxy.cn,direct \
  go build -trimpath -ldflags="-s -w" -o rustdesk-api.new ./cmd/server

# 停止服务、备份旧程序、替换并重启（以 systemd 为例）
sudo systemctl stop rustdesk-api
[ -f /usr/local/bin/rustdesk-api ] && sudo cp /usr/local/bin/rustdesk-api /usr/local/bin/rustdesk-api.bak
sudo mv rustdesk-api.new /usr/local/bin/rustdesk-api
sudo chmod +x /usr/local/bin/rustdesk-api
sudo systemctl start rustdesk-api
sudo systemctl status rustdesk-api --no-pager
```

> 若使用 `pm2` 托管，替换二进制后执行 `pm2 restart rustdesk-api` 即可。

#### 2. 更新官方 RustDesk Server（原生 `hbbs` & `hbbr` 二进制）

更新逻辑：从官方 [rustdesk/rustdesk-server Releases](https://github.com/rustdesk/rustdesk-server/releases) 下载对应架构压缩包 → 停止 `hbbs`/`hbbr` → **仅替换可执行文件，切勿删除工作目录下的 `id_ed25519`、`id_ed25519.pub` 与 `db_v2.sqlite3`** → 按原参数（如 `hbbs -r <公网IP>:21117 -k _`、`hbbr -k _`）重启服务。

```bash
ARCH="amd64"   # ARM64 服务器改为 arm64v8
TMP_DIR="$(mktemp -d)"

# 下载官方最新 Release 压缩包并解压（国内服务器可在 URL 前加 https://gh-proxy.com/）
curl -fL "https://github.com/rustdesk/rustdesk-server/releases/latest/download/rustdesk-server-linux-${ARCH}.zip" \
  -o "${TMP_DIR}/rustdesk-server.zip"
unzip -o "${TMP_DIR}/rustdesk-server.zip" -d "${TMP_DIR}"

# 停止服务（按实际托管方式二选一）
sudo systemctl stop rustdesk-hbbs rustdesk-hbbr
# 若使用 pm2：pm2 stop hbbs hbbr

# 仅替换二进制文件（保留工作目录中的 id_ed25519* 与 db_v2.sqlite3）
sudo install -m 755 "$(find "${TMP_DIR}" -type f -name hbbs | head -n1)" /usr/local/bin/hbbs
sudo install -m 755 "$(find "${TMP_DIR}" -type f -name hbbr | head -n1)" /usr/local/bin/hbbr
rm -rf "${TMP_DIR}"

# 重启服务并检查状态
sudo systemctl start rustdesk-hbbs rustdesk-hbbr
# 若使用 pm2：pm2 restart hbbs hbbr
sudo systemctl status rustdesk-hbbs rustdesk-hbbr --no-pager
```

> Debian / Ubuntu 若当初通过官方 `.deb` 包安装，可从 Releases 页面下载新版 `rustdesk-server-hbbs_*.deb` 和 `rustdesk-server-hbbr_*.deb`，执行 `sudo dpkg -i rustdesk-server-hbbs_*.deb rustdesk-server-hbbr_*.deb` 完成就地升级。

#### 3. 更新托管的官方 Web 客户端（可选）

```bash
./scripts/sync-webclient.sh --build-official master
```

### 五、更新前备份、更新后验证与彻底重装

```bash
# 1. 更新前建议备份配置与持久化数据（含公钥与数据库）
tar -czf "rustdesk-backup-$(date +%F).tar.gz" .env data

# 2. 更新后验证 API 健康状态、容器状态与公钥一致性
curl -fsS http://127.0.0.1:21114/health
docker compose ps
docker logs --tail 50 rustdesk-api
cat ./data/server/id_ed25519.pub
```

> 若需要**清空所有数据并彻底重装**（会重新生成 `id_ed25519` 公钥并清空用户/设备记录，所有客户端需重新导入新 Key），执行 `docker compose down && rm -rf ./data` 后再启动即可。

---

## 切换外部数据库（MySQL / PostgreSQL）

默认使用纯 Go SQLite WAL 模式（持久化在 `./data/api/rustdesk-api.db`）。如需切换外部数据库，修改 `.env` 后执行 `docker compose up -d` 即可自动建表迁移并启用连接池：

```env
# MySQL
DB_TYPE=mysql
DB_DSN=rustdesk:password@tcp(mysql:3306)/rustdesk?charset=utf8mb4&parseTime=True&loc=Local

# PostgreSQL
DB_TYPE=postgres
DB_DSN=host=postgres user=rustdesk password=password dbname=rustdesk port=5432 sslmode=disable
```

---

## HTTPS / WSS 与 Web 客户端托管

1. 将证书放置到 `./nginx/ssl/fullchain.pem` 和 `./nginx/ssl/privkey.pem`。
2. 如需本地托管官方 Flutter Web 客户端，可运行脚本编译或从压缩包同步静态文件到 `./data/webclient`：
   ```bash
   ./scripts/sync-webclient.sh --build-official master
   ```
3. 启动内置 Nginx 反代 Profile：
   ```bash
   docker compose --profile nginx up -d
   ```
   启用后可通过 `https://your.domain.com/_admin/` 访问控制台，通过 `https://your.domain.com/webclient/` 访问 Web 客户端，并将 `.env` 中的 `RUSTDESK_API_SERVER` 更新为 `https://your.domain.com`。

---

## 常见问题与测试

### 能登录/后台看到设备，但连接报错「无法通过注册服务器建立连接」

登录和设备列表上报仅使用 `21114/TCP`（`rustdesk-api`），而真正建立远程连接需要通过官方 `hbbs` / `hbbr`。请按顺序排查：

1. **检查云安全组与宿主机防火墙**：必须同时放行 **`21115/TCP`、`21116/TCP`、`21116/UDP`、`21117/TCP`**（最常见原因是只开了 `21114` 或漏开 `21116/UDP` / `21116/TCP`）。可在本地电脑测试端口连通性：
   ```powershell
   # Windows PowerShell
   Test-NetConnection <你的服务器IP> -Port 21115
   Test-NetConnection <你的服务器IP> -Port 21116
   Test-NetConnection <你的服务器IP> -Port 21117
   ```
2. **确认主控端与被控端都已配置并显示绿色「就绪」**：两端均需填写同一套 ID 服务器（填纯 IP 或 `IP:21116`，不要带 `http://`）、中继服务器与 `Key`（与后台「概览」的 `id_ed25519.pub` 一致；重装清空过 `./data/server` 后需两端同步更新 Key）。
3. **核查 `hbbs` / `hbbr` 状态与 `-r` 中继参数**：
   ```bash
   docker compose ps
   docker inspect --format '{{.Config.Cmd}}' rustdesk-hbbs
   docker logs --tail 50 rustdesk-hbbs
   docker logs --tail 50 rustdesk-hbbr
   ```
   确保两个容器均为 `Up`，且 `hbbs` 的 `-r` 参数为你的公网 IP/域名（如 `x.x.x.x:21117`），而非 `localhost:21117`。
4. **Docker 桥接源 IP 改写排查**：若上述端口全部畅通、两边均显示「就绪」但日志中客户端 IP 全是 `172.x.0.1`，可将 `docker-compose.yml` 中的 `hbbs`/`hbbr` 改为 `network_mode: host`（并移除 `ports:` 和 `networks:`）以规避 Docker NAT 干扰。

### 关于官方 OSS `hbbs` 与 Fork 版 `hbbs` 的说明

- 本项目默认采用**官方原版 `rustdesk/rustdesk-server:latest`** 并启用 `-k _` 公钥强校验，确保第一时间跟进官方安全更新与兼容性修复。
- 官方开源版 `hbbs` 不包含商业 Pro 版或社区 Fork 版的信令层强制登录拦截（`MUST_LOGIN`）。所有账户鉴权、设备权限隔离、地址簿、2FA/邮箱验证、SSO/LDAP 与审计均在 API 层完整实现；如确有信令层拦截需求，将 `.env` 中的 `RUSTDESK_SERVER_IMAGE` 替换为对应镜像即可无缝配合本 API 运行。

### 运行测试

```bash
# 本地具备 Go 1.24+ 环境
go test -v ./...

# 通过 Docker 执行测试阶段
docker build --target test .
```

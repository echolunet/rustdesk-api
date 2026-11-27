# RustDesk API Server

[![CI](https://github.com/echolunet/rustdesk-api/actions/workflows/ci.yml/badge.svg)](https://github.com/echolunet/rustdesk-api/actions/workflows/ci.yml)
![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white)
![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)
![RustDesk](https://img.shields.io/badge/RustDesk-Official%20Server-2563EB)

基于 **Go (Gin + GORM)** 自研的开源 RustDesk 后端 API 与现代化 Web 管理控制台，原生解耦编排**官方 `rustdesk/rustdesk-server:latest`**（`hbbs` + `hbbr`），兼具轻量化、账户与安全管控、2FA/邮箱验证以及官方服务端无痛升级能力。

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
  - 提供交互式一键部署脚本 `scripts/install.sh`：自动识别服务器公网 IP、配置默认帧率（30 / 60 / 120 FPS）、生成 `.env` 并输出后台地址、初始密码与官方公钥。
- **官方服务端解耦 & 一键更新**
  - `hbbs`、`hbbr` 直接采用官方镜像 `rustdesk/rustdesk-server:latest` 并默认开启 `-k _` 公钥强校验，密钥持久化在 `./data/server`。
  - 支持 `./scripts/update.sh` 手动更新或启用 Watchtower 定时自动拉取官方最新版，升级过程不丢失任何密钥或数据。
  - 管理后台内置**官方版本检测中心**，实时展示 `rustdesk/rustdesk` 客户端与 `rustdesk/rustdesk-server` 最新 Release 及下载入口。
- **客户端默认策略下发（帧率 FPS / 画质 / 权限）**
  - 支持在 `.env`（如 `CLIENT_DEFAULT_FPS=60`）与管理后台 **「客户端策略」** 集中配置默认帧率（`5–120 FPS`）、图像质量、首选编解码器（VP9 / VP8 / H.264 / H.265 / AV1）、硬件编解码与会话权限开关，并通过 `/api/heartbeat` 自动下发至客户端。
- **完整兼容 RustDesk 客户端协议**
  - **认证与会话**：`/api/login`、`/api/logout`、`/api/currentUser`（JWT 持久化会话，支持管理员主动踢下线）。
  - **地址簿与标签**：完整支持 Legacy v1（`/api/ab`、`/api/ab/get`）与 Modern v2 个人/共享地址簿（`/api/ab/personal`、`/api/ab/shared/profiles`、`/api/ab/peers`、`/api/ab/tags/:guid`、`/api/ab/peer/*`、`/api/ab/tag/*`、`/api/ab/settings`），双向同步。
  - **设备与权限隔离**：`/api/heartbeat`、`/api/sysinfo`（内置 `peer_id + uuid` 归属校验，拒绝跨 UUID 覆盖设备信息）、`/api/peers`、`/api/devices`、`/api/users`、`/api/device-group/accessible`。
  - **审计上报**：`/api/audit/conn`、`/api/audit/file`、`/api/audit/alarm`。
- **2FA 两步验证 & SMTP 邮箱登录校验**
  - **TOTP 动态验证器**：支持 Google / Microsoft Authenticator / 1Password 等扫码绑定，内置 6 个一次性备用恢复码（`XXXX-XXXX`）。
  - **SMTP 邮箱验证**：支持 `STARTTLS` / `SSL` / 无加密发信与后台发送测试邮件；支持 **关闭全局 / 仅新设备首次验证（自动记录信任设备） / 每次登录验证** 及单用户强制开关。
- **内置离线 Web 管理控制台（`/_admin/`）**
  - 零外部 CDN 依赖（纯原生 HTML/CSS/JS 嵌入单个 Go 二进制），支持浅色 / 深色双主题与移动端抽屉布局。
  - 提供仪表盘、用户与分组、受管设备与设备分组 ACL、个人/共享地址簿与真彩标签、审计与登录日志、2FA/SMTP 安全设置、OIDC/GitHub/Google/LDAP 配置。
  - 自动读取官方 `id_ed25519.pub`，生成**客户端一键配置导入串**、`rustdesk --config` 命令以及**手机端扫码配置二维码**。
- **第三方登录（SSO / LDAP）**
  - 支持通用 **OIDC**（Keycloak、Authentik、Authelia、Okta 等，自动发现 `.well-known/openid-configuration`）、**GitHub**、**Google** 与 **LDAP / Active Directory**（支持在线测试连接）。
- **WebSocket 代理 & Web 客户端托管**
  - 内置 `/ws/id` → `hbbs:21118` 与 `/ws/relay` → `hbbr:21119` 反向代理，附带生产级 `nginx/rustdesk.conf` HTTPS/WSS 配置及官方 Flutter Web 客户端同步脚本。

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

### 方式二：仅 Compose 直接拉取预构建镜像

无需克隆整个仓库或编译源码，仅下载编排与配置模板即可启动：

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

> **提示**：仓库首次推送触发 `Docker Publish` workflow 后，请在 GitHub Packages 将 `rustdesk-api` 的包可见性设为 **Public**，此后任意服务器均可免登录直接 `docker compose pull`。

### 方式三：从源码本地构建

如需自行修改 Go 代码或自定义构建，可克隆仓库并叠加 `docker-compose.build.yml`：

```bash
git clone https://github.com/echolunet/rustdesk-api.git
cd rustdesk-api
cp .env.example .env
# 编辑 .env 填入公网地址与默认参数（国内服务器可设置 GOPROXY=https://goproxy.cn,direct）
chmod +x scripts/*.sh
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

### 3. 放行防火墙 / 安全组端口

| 端口 | 协议 | 用途 |
| :--- | :--- | :--- |
| `21114` | TCP | API 服务与 `/_admin/` Web 管理后台 |
| `21115` | TCP | 官方 `hbbs` NAT 类型检测 |
| `21116` | TCP & UDP | 官方 `hbbs` ID 注册、心跳（UDP）与 TCP 打洞 |
| `21117` | TCP | 官方 `hbbr` 中继流量转发 |
| `21118` | TCP | 官方 `hbbs` WebSocket（使用 Nginx HTTPS 反代时可不对外开放） |
| `21119` | TCP | 官方 `hbbr` WebSocket（使用 Nginx HTTPS 反代时可不对外开放） |

### 4. 获取初始密码与官方公钥

```bash
# 查看首次启动自动生成的管理员账号与随机密码
docker logs rustdesk-api

# 查看官方 hbbs 生成的公钥（也可直接登录后台首页查看/复制）
cat ./data/server/id_ed25519.pub
```

浏览器访问 `http://<你的服务器IP>:21114/_admin/` 登录管理后台并及时修改初始密码。

---

## 客户端配置与扫码导入

登录管理后台 **「概览仪表盘」** 或 **「配置与官方更新」**，支持三种方式配置 RustDesk 客户端：

1. **手机端扫码**：打开 RustDesk App「网络设置」→ 点击右上角扫码图标，直接扫描后台生成的配置二维码。
2. **桌面端一键导入**：复制后台生成的「一键配置导入字符串」，或通过 CLI 静默部署：
   ```bash
   rustdesk --config "<配置导入字符串>"
   ```
3. **手动填写**：按后台展示的 ID 服务器、中继服务器、API 服务器与 Public Key 填入客户端「网络设置」。

---

## 安全功能配置（2FA / 邮箱登录验证）

1. **开启 TOTP 两步验证**：在后台左侧菜单进入 **「安全与验证 (2FA)」** → 点击 **「立即开启 2FA」** → 使用验证器 App 扫描二维码并输入 6 位动态码，同时保存生成的 6 个一次性备用恢复码。
2. **开启 SMTP 邮箱验证**：在同一页面填写 SMTP 服务器（支持 `STARTTLS` / `SSL` / 无加密），先发送测试邮件验证连通性，再选择全局验证策略（**关闭** / **仅新设备首次登录验证** / **每次登录验证**）或前往 **「用户与分组」** 为单个用户单独开启「强制邮箱验证」。
3. **管理员应急重置**：若用户更换或丢失验证器手机，管理员可在 **「用户与分组」** 中点击该用户的 **「重置 2FA」**。

---

## 客户端默认策略下发（帧率 / 画质 / 权限）

首次启动时会从 `.env` 读取默认客户端策略写入数据库，之后随时可在 Web 控制台 **「客户端策略」** 实时修改并保存，系统会自动递增策略版本时间戳，在线客户端在下次 `/api/heartbeat` 心跳（通常 15 秒内）自动拉取生效：

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

## 更新官方服务端与 API

### 手动更新

```bash
./scripts/update.sh                # 仅拉取最新官方 rustdesk/rustdesk-server (hbbs & hbbr)
./scripts/update.sh --pull-api     # 同时拉取最新 GHCR 预构建 rustdesk-api 镜像并重启
./scripts/update.sh --rebuild-api  # 源码部署场景：本地重新构建 rustdesk-api 并更新官方服务端
./scripts/update.sh --all          # 拉取最新代码 + 更新/重启 API + 更新官方服务端
```

Windows PowerShell：
```powershell
.\scripts\update.ps1               # 更新官方 hbbs & hbbr
.\scripts\update.ps1 -PullApi      # 拉取最新 GHCR 预构建 API 镜像并重启
.\scripts\update.ps1 -RebuildApi   # 源码本地重建 API 容器并更新
```

### 自动定时更新官方 `hbbs` & `hbbr`

`docker-compose.yml` 已内置 Watchtower Profile（仅对带 `com.centurylinklabs.watchtower.enable=true` 标签的官方 `hbbs`/`hbbr` 容器生效）：

```bash
docker compose --profile auto-update up -d
```

---

## 切换外部数据库（MySQL / PostgreSQL）

默认使用纯 Go SQLite（持久化在 `./data/api/rustdesk-api.db`）。如需切换外部数据库，修改 `.env` 后执行 `docker compose up -d` 即可自动建表迁移：

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

## 关于官方 OSS `hbbs` 与 Fork 版 `hbbs` 的说明

- 本项目默认采用**官方原版 `rustdesk/rustdesk-server:latest`** 并启用 `-k _` 公钥强校验，确保第一时间跟进官方安全更新与兼容性修复。
- 官方开源版 `hbbs` 不包含商业 Pro 版或社区 Fork 版的信令层强制登录拦截（`MUST_LOGIN`）。所有账户鉴权、设备权限隔离、地址簿、2FA/邮箱验证、SSO/LDAP 与审计均在 API 层完整实现；如确有信令层拦截需求，将 `.env` 中的 `RUSTDESK_SERVER_IMAGE` 替换为对应镜像即可无缝配合本 API 运行。

---

## 运行测试

```bash
# 本地具备 Go 1.24+ 环境
go test -v ./...

# 通过 Docker 执行测试阶段
docker build --target test .
```

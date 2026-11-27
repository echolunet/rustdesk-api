# 自建 RustDesk API 与官方服务端编排方案

这是一个完全自研的 **Go (Gin + GORM)** RustDesk 后端 API 与管理控制台实现，原生编排**官方开源 RustDesk 服务端**（`rustdesk/rustdesk-server:latest`，即 `hbbs` + `hbbr`），兼顾稳定、轻量化、安全性以及官方组件无痛更新能力。

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

### 核心特性

1. **官方组件解耦与无缝更新**
   - `hbbs`、`hbbr` 直接运行官方镜像 `rustdesk/rustdesk-server:latest`，支持一键 `docker compose pull` 或启用 Watchtower 定时自动拉取官方最新版，密钥与配置持久化在 `./data/server`，升级零数据丢失。
   - 后台自带 **官方版本检测中心**（`GET /admin/api/system/updates`），实时对比 `rustdesk/rustdesk` 客户端与 `rustdesk/rustdesk-server` 服务端最新 Release 并提供下载入口。
2. **完整的 RustDesk 客户端协议兼容**
   - 认证与会话：`/api/login`（原生支持 `tfa_check` 两步验证与 `email_check` 邮箱验证码握手）、`/api/logout`、`/api/currentUser`（含 JWT 会话追踪与主动踢下线）。
   - 地址簿与标签：完整支持 **Legacy v1**（`/api/ab`、`/api/ab/get`）与 **Modern v2 个人/共享地址簿**（`/api/ab/personal`、`/api/ab/shared/profiles`、`/api/ab/peers`、`/api/ab/tags/:guid`、`/api/ab/peer/*`、`/api/ab/tag/*`、`/api/ab/settings`）。
   - 设备发现与分组权限：`/api/heartbeat`、`/api/sysinfo`（内置 UUID 防篡改校验，杜绝设备 ID 覆盖伪造）、`/api/sysinfover`、`/api/peers`、`/api/devices`、`/api/users`、`/api/device-group/accessible`。
   - 审计上报：`/api/audit/conn`、`/api/audit/file`、`/api/audit/alarm`。
3. **内置 Web 管理控制台（`/_admin/`）**
   - 零外部 CDN 依赖（内网离线可用），支持浅色/深色主题与移动端响应式布局，打包进单个 Go 二进制。
   - 支持仪表盘、用户与用户组、受管设备与设备分组 ACL、个人/共享地址簿与标签、审计与登录日志、OIDC/GitHub/Google/LDAP 配置、一键生成客户端配置导入串与**移动端扫码配置二维码**。
4. **2FA 两步验证与邮箱登录安全校验**
   - **TOTP 动态口令**：支持 Google/Microsoft Authenticator 扫码绑定、6 位动态验证码校验以及 6 个一次性备用恢复码；管理后台支持两步验证登录与管理员重置用户 2FA。
   - **SMTP 邮箱登录验证**：支持配置 `STARTTLS` / `SSL` / 无加密 SMTP 与发送测试邮件，可选 **关闭 / 仅新设备首次验证（自动记录信任设备） / 每次登录验证** 或按单用户开启。
5. **第三方登录（SSO / LDAP）**
   - 支持 **通用 OIDC**（Keycloak、Authentik、Authelia、Okta 等，自动发现 `.well-known/openid-configuration`）、**GitHub**、**Google**（实现 `/api/login-options`、`/api/oidc/auth`、`/api/oidc/auth-query`、`/api/oidc/callback`）。
   - 支持 **LDAP / Active Directory**（支持 LDAPS 与自签证书跳过选项，后台支持一键连通性测试）。
6. **WebSocket 代理与 Web 客户端托管**
   - 内置 `/ws/id` -> `hbbs:21118` 与 `/ws/relay` -> `hbbr:21119` 反向代理，同时提供生产级 `nginx/rustdesk.conf` HTTPS/WSS 模板。
   - 提供 `./scripts/sync-webclient.sh`，可直接通过 Docker 从官方 `rustdesk/rustdesk` 编译最新 Flutter Web 客户端到 `./data/webclient` 并在 `/webclient/` 托管。

---

## 快速开始

### 1. 准备环境变量

```bash
cp .env.example .env
```

编辑 `.env`，至少修改以下三项为你的公网 IP 或域名：

```env
RUSTDESK_ID_SERVER=your.domain.com:21116
RUSTDESK_RELAY_SERVER=your.domain.com:21117
RUSTDESK_API_SERVER=http://your.domain.com:21114
```

> 国内服务器若构建时拉取 Go 模块较慢，可在 `.env` 中取消/修改 `GOPROXY=https://goproxy.cn,direct`。

### 2. 启动容器编排

```bash
docker compose up -d --build
```

### 3. 放行防火墙 / 云服务器安全组端口

| 端口 | 协议 | 用途 |
| :--- | :--- | :--- |
| `21114` | TCP | API 服务与 `/_admin/` Web 管理后台 |
| `21115` | TCP | 官方 `hbbs` NAT 类型检测 |
| `21116` | TCP & UDP | 官方 `hbbs` ID 注册、心跳（UDP）与 TCP 打洞 |
| `21117` | TCP | 官方 `hbbr` 中继流量转发 |
| `21118` | TCP | 官方 `hbbs` WebSocket（若走 Nginx HTTPS 可仅内网暴露） |
| `21119` | TCP | 官方 `hbbr` WebSocket（若走 Nginx HTTPS 可仅内网暴露） |

### 4. 获取初始管理员密码与官方 Key

```bash
# 查看自动生成的初始管理员账号密码
docker logs rustdesk-api

# 查看官方 hbbs 生成的公钥（也可直接在管理后台首页复制）
cat ./data/server/id_ed25519.pub
```

浏览器访问 `http://<你的服务器IP>:21114/_admin/` 登录控制台并修改初始密码。

---

## 客户端配置与一键导入

在控制台 **「概览仪表盘」** 或 **「服务端配置与官方更新」** 中可以直接复制：
- **ID 服务器 / 中继服务器 / API 服务器 / Public Key** 手动填入 RustDesk 客户端「网络设置」；
- 或直接复制 **一键配置导入字符串**，在客户端设置中导入，或使用命令行静默部署：
  ```bash
  rustdesk --config "<配置导入字符串>"
  ```

---

## 更新官方 RustDesk 与自研 API

### 方式一：手动一键更新（推荐）

Linux / macOS：
```bash
chmod +x scripts/update.sh scripts/sync-webclient.sh
./scripts/update.sh                # 仅拉取最新官方 rustdesk/rustdesk-server (hbbs & hbbr)
./scripts/update.sh --rebuild-api  # 同时从源码重新编译更新自研 API 容器
./scripts/update.sh --all          # 拉取 GitHub 最新源码 + 重建 API + 更新官方服务端
```

Windows PowerShell：
```powershell
.\scripts\update.ps1               # 更新官方 hbbs & hbbr
.\scripts\update.ps1 -RebuildApi   # 同时重建 API 容器
```

### 方式二：自动定时更新官方 `hbbs` & `hbbr`

Compose 中已内置 Watchtower Profile（仅针对打标 `com.centurylinklabs.watchtower.enable=true` 的官方 `hbbs` / `hbbr` 容器生效，不会误删本地自建 API）：

```bash
docker compose --profile auto-update up -d
```

---

## 切换外部数据库（MySQL / PostgreSQL）

默认使用零依赖纯 Go SQLite（持久化在 `./data/api/rustdesk-api.db`）。如需切换为 MySQL 或 PostgreSQL，只需修改 `.env` 后重启 `rustdesk-api`：

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

1. 将 SSL 证书放入 `./nginx/ssl/fullchain.pem` 和 `./nginx/ssl/privkey.pem`。
2. 同步或构建官方 Web 客户端静态文件：
   ```bash
   ./scripts/sync-webclient.sh --build-official master
   ```
3. 启用内置 Nginx 反代 Profile：
   ```bash
   docker compose --profile nginx up -d
   ```
   此时可通过 `https://your.domain.com/_admin/` 访问后台，`https://your.domain.com/webclient/` 访问 Web 客户端，`RUSTDESK_API_SERVER` 可更新为 `https://your.domain.com`。

---

## 关于官方 OSS `hbbs` 与 Fork 版 `hbbs` 的差异说明

- 本项目默认采用 **官方原版 `rustdesk/rustdesk-server:latest`** 并强制开启 `-k _` 公钥加密校验，确保你可以第一时间紧跟官方安全补丁与版本更新。
- 官方开源版 `hbbs` 本身不含闭源 Pro 版的强制登录后才允许信令握手（`MUST_LOGIN`）逻辑（`lejianwen` 方案是通过自行修改 fork 版 `hbbs` 实现的）。在本项目中，所有**用户账号登录、地址簿、标签、设备权限隔离、SSO/LDAP、审计日志**均在 API 层完整实现；若未来确有信令层强登录需求，只需将 `.env` 中 `RUSTDESK_SERVER_IMAGE` 替换为自定义 `hbbs` 镜像即可无缝兼容。

---

## 上传到 GitHub 并在 VPS 拉取部署

> 本仓库已内置 `.gitignore`（自动忽略 `.env`、`data/`、`nginx/ssl/*.pem` 和本地数据库文件）与 `.gitattributes`（强制 `.sh`/源码为 LF 换行符），可直接从 Windows 安全推送到 GitHub。

### 1. 本地推送到你的 GitHub 仓库（Windows）

```powershell
cd e:\workspace\rustdesk
git init -b main
git add .
git commit -m "init: rustdesk custom api and official server compose"
git remote add origin https://github.com/<你的用户名>/<你的仓库名>.git
git push -u origin main
```

### 2. 在 VPS 上首次拉取并启动

```bash
git clone https://github.com/<你的用户名>/<你的仓库名>.git rustdesk
cd rustdesk
cp .env.example .env
# 修改 .env 中的公网 IP/域名 (RUSTDESK_ID_SERVER / RUSTDESK_RELAY_SERVER / RUSTDESK_API_SERVER)
chmod +x scripts/update.sh scripts/sync-webclient.sh
docker compose up -d --build
```

### 3. 日后 VPS 一键同步更新（你的仓库 + 官方最新版）

```bash
./scripts/update.sh --all
```

---

## 运行自动化测试

```bash
# 在本地有 Go 1.24+ 环境时：
go test -v ./...

# 无本地 Go 环境时，直接通过 Docker 执行测试阶段：
docker build --target test .
```

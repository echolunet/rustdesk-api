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

- **官方服务端解耦 & 一键更新**
  - `hbbs`、`hbbr` 直接采用官方镜像 `rustdesk/rustdesk-server:latest` 并默认开启 `-k _` 公钥强校验，密钥持久化在 `./data/server`。
  - 支持 `./scripts/update.sh` 手动更新或启用 Watchtower 定时自动拉取官方最新版，升级过程不丢失任何密钥或数据。
  - 管理后台内置**官方版本检测中心**，实时展示 `rustdesk/rustdesk` 客户端与 `rustdesk/rustdesk-server` 最新 Release 及下载入口。
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

### 1. 克隆项目并配置环境变量

```bash
git clone https://github.com/echolunet/rustdesk-api.git
cd rustdesk-api
cp .env.example .env
```

编辑 `.env`，将以下三项修改为你的服务器公网 IP 或域名：

```env
RUSTDESK_ID_SERVER=your.domain.com:21116
RUSTDESK_RELAY_SERVER=your.domain.com:21117
RUSTDESK_API_SERVER=http://your.domain.com:21114
```

> 国内服务器若构建时拉取 Go 依赖较慢，可在 `.env` 中设置 `GOPROXY=https://goproxy.cn,direct`。

### 2. 启动服务

```bash
chmod +x scripts/update.sh scripts/sync-webclient.sh
docker compose up -d --build
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

## 更新官方服务端与 API

### 手动更新

```bash
./scripts/update.sh                # 仅拉取最新官方 rustdesk/rustdesk-server (hbbs & hbbr)
./scripts/update.sh --rebuild-api  # 重建本地 rustdesk-api 容器并更新官方服务端
./scripts/update.sh --all          # 拉取最新代码 + 重建 API + 更新官方服务端
```

Windows PowerShell：
```powershell
.\scripts\update.ps1               # 更新官方 hbbs & hbbr
.\scripts\update.ps1 -RebuildApi   # 同时重建 API 容器
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

# wecom-nezha

一个将企业微信与哪吒监控深度集成的服务，同时支持 Telegram Bot。支持企业微信消息推送（文本/Markdown/图片）、邮件发送，在企业微信/Telegram 中直接查询服务器状态、管理 NAT 穿透、获取安装命令、远程重启，支持明文/加密回调模式，开箱即用的 Docker 部署方案。

## 功能特性

- ✅ **企业微信消息推送**：文本、Markdown、图片、链接，支持 Header/Query 认证（Webhook 风格）
- ✅ **企业微信邮件发送**：支持抄送、密送、附件
- ✅ **Telegram Bot 集成**：
  - Inline Keyboard 交互：服务器列表点击查看详情
  - Callback Query：一键确认操作
  - 命令支持：`/status`、`/list`、`/offline`、`/service`、`/help`
  - 复用所有哪吒监控命令
- ✅ **哪吒监控集成**：
  - 接收企业微信/Telegram 消息，查询服务器状态
  - 支持关键词：`状态`、`离线`、`列表`、`帮助`
  - 支持服务器名称精确/模糊查询
  - 支持 Agent 安装命令（Linux / Windows / Docker）
  - 支持服务器重启（自动识别平台）
- ✅ **NAT 穿透管理**：
  - 查看、添加、删除、启用/禁用 NAT 配置
  - 添加时分步引导，删除/修改需确认
- ✅ **DDNS 管理**：
  - 查看、添加、删除、启用/禁用 DDNS 配置
  - 支持查看提供商列表，分步引导添加
- ✅ **通知渠道管理**：
  - 查看、添加、删除通知渠道
  - 支持快速添加和分步引导两种模式
- ✅ **企业微信回调**：支持明文模式和加密模式
- ✅ **健康检查**：`/healthz`（存活）、`/readyz`（就绪）
- ✅ **Docker 部署**：支持多架构构建（amd64、arm64）

## 快速开始

### 1. 创建 Telegram Bot（可选）

1. 打开 Telegram，搜索 [@BotFather](https://t.me/BotFather)
2. 发送 `/newbot`
3. 按提示设置：
   - Bot 名称（如 `Nezha 监控 Bot`）
   - Bot 用户名（如 `nezha_monitor_bot`，必须以 `bot` 结尾）
4. 获取 Token（格式类似 `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`）

### 2. Docker Compose（推荐）

```yaml
services:
  wecom-nezha:
    image: gowfqk/wecom-nezha:latest
    container_name: wecom-nezha
    ports:
      - "8080:8080"
    environment:
      - SENDKEY=${SENDKEY:-set_a_sendkey}
      - WECOM_CID=${WECOM_CID}
      - WECOM_SECRET=***      - WECOM_AID=${WECOM_AID}
      - WECOM_TOUID=${WECOM_TOUID:-@all}
      - WECOM_TOKEN=***      - WECOM_ENCODING_AES_KEY=${WECOM_ENCODING_AES_KEY}
      - NEZHA_URL=${NEZHA_URL}
      - NEZHA_USERNAME=${NEZHA_USERNAME}
      - NEZHA_PASSWORD=${NEZH...D}
      - CACHE_TYPE=${CACHE_TYPE:-memory}
      - REDIS_STAT=${REDIS_STAT:-OFF}
      - REDIS_ADDR=${REDIS_ADDR:-redis:6379}
      - REDIS_PASSWORD=${REDI...D}
      - TELEGRAM_BOT_TOKEN=${TELE...N}
      - TELEGRAM_WEBHOOK_SECRET=${TELE...T}
      - TELEGRAM_ALLOWED_USERS=${TELEGRAM_ALLOWED_USERS}
    volumes:
      - ./data:/data
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 10s
```

### 3. Docker 运行

```bash
docker run -d -p 8080:8080 \
  -e SENDKEY=your_sendkey \
  -e WECOM_CID=your_corpid \
  -e WECOM_SECRET=*** \
  -e WECOM_AID=your_agentid \
  -e WECOM_TOKEN=your_c...oken \
  -e WECOM_ENCODING_AES_KEY=your_aes_key \
  -e NEZHA_URL=https://nezha.example.com \
  -e NEZHA_USERNAME=admin \
  -e NEZHA_PASSWORD=*** \
  -e CACHE_TYPE=memory \
  -e TELEGRAM_BOT_TOKEN=your_t...oken \
  -e TELEGRAM_WEBHOOK_SECRET=your_w...cret \
  -e TELEGRAM_ALLOWED_USERS=123456789,987654321 \
  gowfqk/wecom-nezha:latest
```

### 4. 设置 Telegram Webhook

```bash
# 设置 Webhook
curl -X POST "https://api.telegram.org/bot<YOUR_TOKEN>/setWebhook" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://your-domain.com/telegram/webhook"}'

# 验证 Webhook
curl "https://api.telegram.org/bot<YOUR_TOKEN>/getWebhookInfo"
```

## API 接口

### 发送消息 - `/wecomchan`

**请求方式**：`POST`

**Content-Type**: `application/json`

**认证方式**（任选一种）：
1. Body / Form 中的 `sendkey` 或 `token` 字段
2. Header: `X-Webhook-Token`
3. Query 参数: `?token=xxx`

**请求参数**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `sendkey` / `token` | string | ✅ | 认证密钥（与 SENDKEY 一致） |
| `msg` / `content` / `text.content` | string | ✅ | 消息内容（任选一种格式） |
| `msg_type` | string | ❌ | 消息类型：`text`（默认）、`markdown`、`image` |
| `title` | string | ❌ | 消息标题，自动拼接为 `【标题】` 前缀 |
| `touser` | string | ❌ | 接收人，默认 `@all` |
| `agentid` | string | ❌ | 应用 ID，默认使用环境变量 |

**请求示例**：

```bash
# 标准格式（text.content）
curl -X POST https://your-domain.com/wecomchan \
  -H "Content-Type: application/json" \
  -d '{
    "sendkey": "your_sendkey",
    "msg_type": "text",
    "text": {"content": "这是一条测试消息"}
  }'

# Webhook 风格（token + content + title）
curl -X POST https://your-domain.com/wecomchan \
  -H "Content-Type: application/json" \
  -d '{
    "token": "your_sendkey",
    "title": "Nezha 告警",
    "content": "服务器 CPU 使用率超过 90%",
    "msg_type": "markdown"
  }'

# Header 认证
curl -X POST https://your-domain.com/wecomchan \
  -H "Content-Type: application/json" \
  -H "X-Webhook-Token: your_sendkey" \
  -d '{"content":"部署完成！"}'

# 指定接收人
curl -X POST https://your-domain.com/wecomchan \
  -H "Content-Type: application/json" \
  -d '{"token":"your_sendkey","content":"运维通知","touser":"zhangsan|lisi"}'
```

**Nezha 告警集成**：

在哪吒监控面板 → 通知 → 添加通知方式中选择 **Webhook**，配置：

- URL: `https://your-domain.com/wecomchan`
- 请求方式: `POST`
- 请求头: `Content-Type: application/json`
- 请求体模板:

```json
{
  "token": "your_sendkey",
  "title": "Nezha 告警",
  "content": "#%%URL#%% 在 #%%DATE#%% 发生异常，当前状态：#%%TRIGGER_TYPE#%%",
  "msg_type": "markdown"
}
```

---

### 发送邮件 - `/mail`

**请求方式**：`POST`

**Content-Type**: `application/json`

**请求体示例**：

```json
{
  "sendkey": "your_sendkey",
  "to": {
    "emails": ["user@example.com"]
  },
  "subject": "邮件主题",
  "content": "邮件内容",
  "content_type": "text"
}
```

**可选字段**：`cc`（抄送）、`bcc`（密送）、`attachment_list`（附件）、`enable_id_trans`（userid转openid）

### 企业微信回调 - `/callback`

**请求方式**：`GET`（验证回调URL）、`POST`（接收消息）

用于接收企业微信消息，并提供哪吒监控服务器状态查询功能。

**配置步骤**：
1. 在企业微信后台配置回调URL为 `https://your-domain.com/callback`
2. 设置回调Token和EncodingAESKey
3. 配置环境变量 `WECOM_TOKEN`（回调Token）
4. 如使用加密模式，还需配置 `WECOM_ENCODING_AES_KEY`（EncodingAESKey）

---

### Telegram Bot - `/telegram/webhook`

**请求方式**：`POST`

用于接收 Telegram 消息和回调查询，提供哪吒监控服务器状态查询和管理功能。

**配置步骤**：
1. 向 [@BotFather](https://t.me/BotFather) 创建 Bot，获取 Token
2. 配置环境变量 `TELEGRAM_BOT_TOKEN`
3. （可选）配置 `TELEGRAM_WEBHOOK_SECRET` 增强安全性
4. （可选）配置 `TELEGRAM_ALLOWED_USERS` 限制访问用户（逗号分隔的用户ID）
5. 设置 Webhook URL：`https://your-domain.com/telegram/webhook`

**支持的命令**：

| 命令 | 说明 |
|------|------|
| `/start` / `/help` | 显示帮助信息 |
| `/status` / `状态` | 查看服务器在线状态摘要 |
| `/list` / `列表` | 查看所有服务器（带 Inline Keyboard） |
| `/offline` / `离线` | 查看离线服务器列表 |
| `/service` / `服务` | 查看服务监控状态 |
| `<服务器名>` | 查询指定服务器详情（支持模糊匹配） |
| `详情 <服务器名>` | 查看服务器完整信息 |
| `重启 <服务器名>` | 重启服务器（需确认） |
| `安装 linux/windows/docker` | 获取 Agent 安装命令 |
| `NAT` / `DDNS` / `通知` | 管理功能 |

**Inline Keyboard 交互**：
- `/list` 命令会显示服务器列表键盘，点击可查看详情
- 底部操作按钮：状态概览、离线列表、刷新、帮助
- 确认/取消操作使用 Inline Keyboard 按钮

### 健康检查

- **存活检查**: `GET /healthz`
- **就绪检查**: `GET /readyz`

## 环境变量配置

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SENDKEY` | 验证密钥 | `set_a_sendkey` |
| `WECOM_CID` | 企业微信公司 ID | - |
| `WECOM_SECRET` | 企业微信应用 Secret | - |
| `WECOM_AID` | 企业微信应用 ID | - |
| `WECOM_TOUID` | 默认接收人 | `@all` |
| `WECOM_TOKEN` | 企业微信回调 Token | - |
| `WECOM_ENCODING_AES_KEY` | 企业微信回调 EncodingAESKey（加密模式） | - |
| `NEZHA_URL` | 哪吒监控面板地址 | - |
| `NEZHA_USERNAME` | 哪吒用户名 | - |
| `NEZHA_PASSWORD` | 哪吒密码 | - |
| `CACHE_TYPE` | 缓存类型：`none`/`memory`/`redis`（推荐 `memory`） | `none` |
| `REDIS_STAT` | Redis 状态：`ON`/`OFF` | `OFF` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `REDIS_PASSWORD` | Redis 密码 | - |
| `MAIL_FOOTER_URL` | 邮件底部链接地址 | - |
| `TELEGRAM_BOT_TOKEN` | Telegram Bot Token | - |
| `TELEGRAM_WEBHOOK_SECRET` | Telegram Webhook 验证密钥 | - |
| `TELEGRAM_ALLOWED_USERS` | 允许访问的 Telegram 用户 ID（逗号分隔） | 空表示允许所有 |

## 项目结构

```
.
├── main.go             # 程序入口，路由注册
├── config.go           # 配置和全局变量
├── types.go            # 结构体定义
├── utils.go            # 工具函数
├── utils_test.go       # 工具函数测试
├── wecom_api.go        # 企业微信 API 封装
├── nezha_api.go        # 哪吒监控 API 封装（服务器/NAT/DDNS/通知）
├── handlers.go         # HTTP 处理器（/wecomchan, /webhook, /mail）
├── callback.go         # 企业微信回调处理（明文/加密模式）
├── telegram.go         # Telegram Bot 处理（Webhook/Inline Keyboard）
├── .env.example        # 环境变量示例
├── docker-compose.yml  # Docker Compose 配置
├── Dockerfile
├── docs/               # 文档
└── .github/            # GitHub Actions CI/CD
```

## 构建

```bash
# 本地构建
go build

# Docker 构建
docker build -t gowfqk/wecom-nezha:latest .
```

## 许可证

MIT

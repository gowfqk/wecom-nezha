# wecom-nezha

一个将企业微信与哪吒监控深度集成的服务。支持企业微信消息推送（文本/Markdown/图片）、邮件发送，在企业微信中直接查询服务器状态、管理 NAT 穿透、获取安装命令、远程重启，支持明文/加密回调模式，开箱即用的 Docker 部署方案。

## 功能特性

- ✅ **企业微信消息推送**：文本、Markdown、图片、链接，支持 Header/Query 认证（Webhook 风格）
- ✅ **企业微信邮件发送**：支持抄送、密送、附件
- ✅ **哪吒监控集成**：
  - 接收企业微信消息，查询服务器状态
  - 支持关键词：`状态`、`离线`、`列表`、`帮助`
  - 支持服务器名称精确/模糊查询
  - 支持 Agent 安装命令（Linux / Windows / Docker）
  - 支持服务器重启（自动识别平台）
- ✅ **NAT 穿透管理**：
  - 查看、添加、删除、启用/禁用 NAT 配置
  - 添加时分步引导，删除/修改需确认
- ✅ **企业微信回调**：支持明文模式和加密模式
- ✅ **健康检查**：`/healthz`（存活）、`/readyz`（就绪）
- ✅ **Docker 部署**：支持多架构构建（amd64、arm64）

## 快速开始

### Docker Compose（推荐）

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
      - WECOM_SECRET=${WECOM_SECRET}
      - WECOM_AID=${WECOM_AID}
      - WECOM_TOUID=${WECOM_TOUID:-@all}
      - WECOM_TOKEN=${WECOM_TOKEN}
      - WECOM_ENCODING_AES_KEY=${WECOM_ENCODING_AES_KEY}
      - NEZHA_URL=${NEZHA_URL}
      - NEZHA_USERNAME=${NEZHA_USERNAME}
      - NEZHA_PASSWORD=${NEZHA_PASSWORD}
      - CACHE_TYPE=${CACHE_TYPE:-memory}
      - REDIS_STAT=${REDIS_STAT:-OFF}
      - REDIS_ADDR=${REDIS_ADDR:-redis:6379}
      - REDIS_PASSWORD=${REDIS_PASSWORD}
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

### Docker 运行

```bash
docker run -d -p 8080:8080 \
  -e SENDKEY=your_sendkey \
  -e WECOM_CID=your_corpid \
  -e WECOM_SECRET=your_secret \
  -e WECOM_AID=your_agentid \
  -e WECOM_TOKEN=your_callback_token \
  -e WECOM_ENCODING_AES_KEY=your_aes_key \
  -e NEZHA_URL=https://nezha.example.com \
  -e NEZHA_USERNAME=admin \
  -e NEZHA_PASSWORD=your_password \
  -e CACHE_TYPE=memory \
  gowfqk/wecom-nezha:latest
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

**支持的命令**：

| 命令 | 说明 |
|------|------|
| `帮助` / `help` / `?` | 显示帮助信息 |
| `状态` / `状态查询` | 查看服务器在线状态摘要 |
| `离线` | 查看离线服务器列表 |
| `列表` / `list` | 查看所有服务器 |
| `<服务器名>` | 查询指定服务器详情（支持模糊匹配） |
| `详情 <服务器名>` | 查看服务器完整信息（CPU、内存、磁盘、负载、网络等） |
| `安装 linux` | 获取 Linux Agent 一键安装命令 |
| `安装 windows` | 获取 Windows Agent 安装命令 |
| `安装 docker` | 获取 Docker Agent 安装命令 |
| `重启 <服务器名>` | 重启服务器（需确认，自动识别 Linux/Windows） |
| `NAT` / `NAT 列表` | 查看 NAT 穿透配置列表 |
| `NAT 添加` | 分步添加 NAT 穿透配置 |
| `NAT 添加 名称 域名 内网地址 服务器名` | 一步完成添加 |
| `NAT 启用 <ID>` | 启用 NAT 配置 |
| `NAT 禁用 <ID>` | 禁用 NAT 配置 |
| `NAT 删除 <ID>` | 删除 NAT 配置（需确认） |
| `NAT 修改 <ID> <内网地址:端口> [服务器名]` | 修改 NAT 配置（地址和/或服务器） |
| `NAT 修改 <ID> - <服务器名>` | 只修改 NAT 的服务器 |
| `标签 <服务器名> <标签内容>` | 更新服务器私有备注/标签 |
| `确认` / `取消` | 确认或取消待执行的操作 |

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
| `CACHE_TYPE` | 缓存类型：`none`/`memory`/`redis` | `none` |
| `REDIS_STAT` | Redis 状态：`ON`/`OFF` | `OFF` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `REDIS_PASSWORD` | Redis 密码 | - |
| `MAIL_FOOTER_URL` | 邮件底部链接地址 | - |

## 项目结构

```
.
├── main.go             # 程序入口，路由注册
├── config.go           # 配置和全局变量
├── types.go            # 结构体定义
├── utils.go            # 工具函数
├── utils_test.go       # 工具函数测试
├── wecom_api.go        # 企业微信 API 封装
├── nezha_api.go        # 哪吒监控 API 封装
├── handlers.go         # HTTP 处理器（/wecomchan, /webhook, /mail）
├── callback.go         # 企业微信回调处理（明文/加密模式）
├── wecomchan.go        # 遗留入口文件（已重构）
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

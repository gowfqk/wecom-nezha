# go-wecomchan

通过企业微信向微信推送消息的 Go 语言解决方案。

> 本项目基于 [wecomchan](https://github.com/easychen/wecomchan) 重构而来。

## 功能特性

- ✅ **多消息类型**：文本（text）、Markdown（markdown）、图片（image）、链接（link）、小程序（miniprogram）
- ✅ **多接口设计**：
  - `/wecomchan` - 企业内部应用消息
  - `/mail` - 邮件推送消息
- ✅ **灵活传参**：Body JSON 传参、URL 参数传参
- ✅ **多重缓存**：不缓存、内存缓存、Redis 缓存
- ✅ **健康检查**：`/healthz`（存活）、`/readyz`（就绪）
- ✅ **Docker 部署**：支持多架构构建（amd64、arm64）
- ✅ **代码重构**：模块化结构，带单元测试

## 快速开始

### Docker Compose（推荐）

```yaml
services:
  wecomchan:
    image: gowfqk/go-wecomchan:latest
    container_name: wecomchan
    ports:
      - "8080:8080"
    environment:
      - SENDKEY=your_sendkey
      - WECOM_CID=your_corpid
      - WECOM_SECRET=your_secret
      - WECOM_AID=your_agentid
      - WECOM_TOUID=@all
      - CACHE_TYPE=memory
    restart: unless-stopped
```

### Docker 运行

```bash
docker run -d -p 8080:8080 \
  -e SENDKEY=your_sendkey \
  -e WECOM_CID=your_corpid \
  -e WECOM_SECRET=your_secret \
  -e WECOM_AID=your_agentid \
  -e CACHE_TYPE=memory \
  gowfqk/go-wecomchan:latest
```

### 本地运行

```bash
# 设置环境变量
export SENDKEY=your_sendkey
export WECOM_CID=your_corpid
export WECOM_SECRET=your_secret
export WECOM_AID=your_agentid
export CACHE_TYPE=memory

# 运行
go run .
```

## API 接口

### 企业内部消息 - `/wecomchan`

**请求方式**：`POST`

**Content-Type**: `application/json`

**请求体示例**：

```json
{
  "sendkey": "your_sendkey",
  "msg_type": "text",
  "text": {
    "content": "这是一条测试消息"
  }
}
```

### 邮件推送 - `/mail`

**请求方式**：`POST`

**Content-Type**: `application/json`

**请求体示例**：

```json
{
  "sendkey": "your_sendkey",
  "to": {
    "emails": ["user@example.com"],
    "userids": ["william"]
  },
  "cc": {
    "emails": ["cc@example.com"],
    "userids": ["panyy"]
  },
  "bcc": {
    "emails": ["bcc@example.com"]
  },
  "subject": "邮件主题",
  "content": "邮件正文内容",
  "content_type": "html",
  "attachment_list": [
    {
      "file_name": "a.txt",
      "content": "BASE64_CONTENT"
    }
  ],
  "enable_id_trans": 1
}
```

**参数说明**：

| 参数 | 必填 | 说明 |
|------|------|------|
| `sendkey` | 是 | 验证密钥 |
| `to` | 是 | 收件人对象，含 `emails`（邮箱数组）或 `userids`（企业成员userid数组），至少传一个 |
| `to.emails` | 否 | 收件人邮箱地址数组 |
| `to.userids` | 否 | 收件人企业成员userid数组 |
| `cc` | 否 | 抄送人对象，格式同 `to` |
| `bcc` | 否 | 密送人对象，格式同 `to` |
| `subject` | 是 | 邮件主题 |
| `content` | 是 | 邮件正文 |
| `content_type` | 否 | 内容类型：`html`（默认）或 `text` |
| `attachment_list` | 否 | 附件列表 |
| `attachment_list[].file_name` | 否 | 文件名 |
| `attachment_list[].content` | 否 | 文件内容（base64编码） |
| `enable_id_trans` | 否 | 是否开启id转译：`0`（默认）或 `1` |

### 健康检查

- **存活检查**: `GET /healthz`
- **就绪检查**: `GET /readyz`（会验证 access_token 是否可获取）

## 环境变量配置

| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `SENDKEY` | 验证密钥 | `set_a_sendkey` |
| `WECOM_CID` | 企业微信公司 ID | - |
| `WECOM_SECRET` | 企业微信应用 Secret | - |
| `WECOM_AID` | 企业微信应用 ID | - |
| `WECOM_TOUID` | 默认接收人 | `@all` |
| `CACHE_TYPE` | 缓存类型：`none`/`memory`/`redis` | `none` |
| `REDIS_STAT` | Redis 开关：`ON`/`OFF` | `OFF` |
| `REDIS_ADDR` | Redis 地址 | `localhost:6379` |
| `REDIS_PASSWORD` | Redis 密码 | - |
| `MAIL_FOOTER_URL` | 邮件底部访问链接 | - |

### Redis 缓存配置

当 `CACHE_TYPE` 设置为 `redis` 时，系统会使用 Redis 缓存 access_token。需要额外配置：

| 场景 | 配置 |
|------|------|
| 单机部署 | `REDIS_STAT=ON`，`REDIS_ADDR=localhost:6379` |
| 集群部署 | `REDIS_STAT=ON`，`REDIS_ADDR=redis-host:6379`，`REDIS_PASSWORD=your_password` |

**注意**：使用 Redis 缓存时，必须将 `REDIS_STAT` 设置为 `ON`，否则即使 `CACHE_TYPE=redis` 也不会启用 Redis。

## 项目结构

```
.
├── config.go           # 配置和全局变量
├── types.go            # 结构体定义
├── utils.go            # 工具函数
├── utils_test.go       # 单元测试
├── wecom_api.go        # 企业微信 API 封装
├── handlers.go         # HTTP 处理器
├── main.go             # 程序入口
├── wecomchan.go        # 原入口（已废弃，保留兼容）
├── docker-compose.yml  # Docker Compose 配置
└── Dockerfile
```

## 测试

```bash
# 运行单元测试
go test -v

# 运行基准测试
go test -bench=.
```

## 构建

```bash
# 本地构建
go build

# Docker 构建
docker build -t gowfqk/go-wecomchan:latest .
```

## 许可证

MIT

---

> **鸣谢**：本项目基于 [wecomchan](https://github.com/easychen/wecomchan) 开发，感谢原作者。
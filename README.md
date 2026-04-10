# wecom-nezha

企业微信推送 + 哪吒监控集成解决方案。

## 功能特性

- ✅ **企业微信消息推送**：文本、Markdown、图片、链接
- ✅ **哪吒监控集成**：
  - 接收企业微信消息，查询服务器状态
  - 支持关键词：`状态`、`离线`、`列表`、`帮助`
  - 支持服务器名称精确/模糊查询
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
      - SENDKEY=your_sendkey
      - WECOM_CID=your_corpid
      - WECOM_SECRET=your_secret
      - WECOM_AID=your_agentid
      - WECOM_TOUID=@all
      - WECOM_TOKEN=your_callback_token
      - NEZHA_URL=https://nezha.example.com
      - NEZHA_USERNAME=admin
      - NEZHA_PASSWORD=your_password
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
  -e WECOM_TOKEN=your_callback_token \
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

### 企业微信回调 - `/callback`

**请求方式**：`GET`（验证回调URL）、`POST`（接收消息）

用于接收企业微信消息，并提供哪吒监控服务器状态查询功能。

**配置步骤**：
1. 在企业微信后台配置回调URL为 `https://your-domain.com/callback`
2. 设置回调Token和EncodingAESKey
3. 配置环境变量 `WECOM_TOKEN`

**支持的命令**：

| 命令 | 说明 |
|------|------|
| `帮助` / `help` / `?` | 显示帮助信息 |
| `状态` / `状态查询` | 查看服务器在线状态摘要 |
| `离线` | 查看离线服务器列表 |
| `列表` / `list` | 查看所有服务器 |
| `<服务器名>` | 查询指定服务器详情（支持模糊匹配） |

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
| `WECOM_TOKEN` | 企业微信回调Token | - |
| `WECOM_ENCODING_AES_KEY` | 企业微信EncodingAESKey（加密模式） | - |
| `NEZHA_URL` | 哪吒监控面板地址 | - |
| `NEZHA_USERNAME` | 哪吒用户名 | - |
| `NEZHA_PASSWORD` | 哪吒密码 | - |
| `CACHE_TYPE` | 缓存类型：`none`/`memory`/`redis` | `none` |

## 项目结构

```
.
├── config.go           # 配置和全局变量
├── types.go            # 结构体定义
├── utils.go            # 工具函数
├── wecom_api.go        # 企业微信 API 封装
├── nezha_api.go        # 哪吒监控 API 封装
├── handlers.go         # HTTP 处理器
├── callback.go         # 回调接口处理
├── main.go             # 程序入口
├── docker-compose.yml  # Docker Compose 配置
└── Dockerfile
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

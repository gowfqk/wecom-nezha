# 架构设计文档

> wecom-nezha 系统架构与数据流说明

---

## 一、整体架构

```
┌─────────────────────────────────────────────────────────────────────────┐
│                          用户 / 外部系统                                  │
├──────────────┬──────────────┬──────────────┬────────────────────────────┤
│  企业微信APP  │  Webhook调用  │  邮件客户端   │  哪吒监控告警              │
│  (聊天交互)   │  (消息推送)   │  (收件人)     │  (触发通知)               │
└──────┬───────┴──────┬───────┴──────┬───────┴────────────┬───────────────┘
       │              │              │                    │
       │ 回调消息     │ POST请求     │                    │ Webhook POST
       ▼              ▼              │                    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                     wecom-nezha 服务 (:8080)                             │
│                                                                          │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐ │
│  │/callback │  │/wecomchan│  │  /mail   │  │ /healthz │  │ /readyz  │ │
│  │ 回调处理  │  │ 消息推送  │  │ 邮件发送  │  │ 存活检查  │  │ 就绪检查  │ │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘  └──────────┘  └──────────┘ │
│       │              │              │                                     │
│       ▼              ▼              ▼                                     │
│  ┌──────────────────────────────────────────┐                           │
│  │           wecom_api.go                    │                           │
│  │  - GetAccessToken (Token 管理)            │                           │
│  │  - PostMsg (发送应用消息)                  │                           │
│  │  - UploadMedia (图片上传)                  │                           │
│  │  - SendMailMessage (邮件发送)              │                           │
│  └──────────────────┬───────────────────────┘                           │
│       │             │                                                    │
│       │             ▼                                                    │
│       │  ┌──────────────────────┐                                       │
│       │  │   Token 缓存层       │                                       │
│       │  │  Redis / Memory      │                                       │
│       │  └──────────────────────┘                                       │
│       │                                                                  │
│       ▼                                                                  │
│  ┌──────────────────────────────────────────┐                           │
│  │           nezha_api.go                    │                           │
│  │  - NezhaLogin (JWT 登录/刷新)             │                           │
│  │  - GetNezhaServerList (服务器列表)         │                           │
│  │  - RebootNezhaServer (远程重启)            │                           │
│  │  - NAT 管理 (CRUD)                        │                           │
│  │  - GetServerMetrics (监控历史)             │                           │
│  └──────────────────────────────────────────┘                           │
└─────────────────────────────────────────────────────────────────────────┘
       │                              │
       ▼                              ▼
┌──────────────┐             ┌──────────────────┐
│ 企业微信 API  │             │  哪吒监控 API     │
│ qyapi.wx.qq  │             │  /api/v1/*       │
└──────────────┘             └──────────────────┘
```

---

## 二、组件职责

| 组件 | 文件 | 职责 |
|------|------|------|
| **程序入口** | `main.go` | 路由注册、HTTP Server 配置 |
| **全局配置** | `config.go` | 环境变量读取、全局常量、缓存结构 |
| **数据类型** | `types.go` | 所有请求/响应结构体定义 |
| **工具函数** | `utils.go` | JSON 解析、中间件、验证、格式化 |
| **企微 API** | `wecom_api.go` | Token 获取/缓存、消息发送、媒体上传 |
| **哪吒 API** | `nezha_api.go` | 登录认证、服务器管理、NAT 管理、监控数据 |
| **HTTP 处理** | `handlers.go` | `/wecomchan`、`/mail`、健康检查处理器 |
| **回调处理** | `callback.go` | 企微回调验证/解密、聊天命令路由与执行 |

---

## 三、核心数据流

### 3.1 消息推送流程（/wecomchan）

```
外部系统 → POST /wecomchan (sendkey 认证)
    → handlers.go: wecomChan()
        → 验证 sendkey
        → 获取 AccessToken (缓存优先)
        → 根据 msg_type 处理:
            - text/markdown: 构造 JsonData
            - image: 先 UploadMedia 获取 media_id
        → PostMsg() 发送到企微 API
        → 返回企微 API 响应
```

### 3.2 聊天式运维流程（/callback）

```
用户在企微发消息 → 企微推送到 /callback
    → callback.go: WecomCallbackHandler()
        → GET: 验证回调 URL (明文/加密签名)
        → POST: 处理消息
            → 签名验证
            → 解密（加密模式）
            → 消息去重（MsgId 5分钟缓存）
            → processUserMessage() 命令路由:
                ├── "状态" → GetNezhaServerList() → 统计在线/离线
                ├── "列表" → GetNezhaServerList() → 格式化列表
                ├── "详情 X" → GetNezhaServerByName() → 完整信息
                ├── "重启 X" → 保存 pendingAction → 等待确认
                ├── "NAT ..." → NAT CRUD 操作
                ├── "监控 X" → GetServerMetrics() → 历史数据
                ├── "服务" → GetServiceList() → 服务状态
                └── 其他 → 模糊匹配服务器名
            → sendReplyMessage() → 通过企微 API 回复用户
```

### 3.3 Token 管理策略

```
请求需要 Token
    → GetAccessToken()
        → 检查缓存 (Redis/Memory)
            ├── 命中: 返回缓存 Token
            └── 未命中: GetRemoteToken()
                → 调用企微 gettoken API
                → 写入缓存 (7000秒 TTL)
                → 返回新 Token
    → 使用 Token 调用 API
        → 如果返回 errcode=42001 (Token过期)
            → ValidateToken() 清除缓存
            → 重试 (最多3次)
```

### 3.4 哪吒 Token 管理

```
需要调用哪吒 API
    → NezhaLogin()
        → 检查 nezhaAccessToken 是否有效（提前5分钟过期）
            ├── 有效: 直接使用
            └── 过期/为空:
                → 尝试 RefreshNezhaToken() (GET /refresh-token)
                    ├── 成功: 更新 token 和过期时间
                    └── 失败: 重新登录 (POST /login)
        → 设置 Authorization: Bearer <token>
    → 发起实际 API 请求
```

---

## 四、安全机制

| 机制 | 实现位置 | 说明 |
|------|----------|------|
| Sendkey 认证 | `handlers.go` | 所有推送 API 需验证 sendkey |
| 签名验证 | `callback.go` | SHA1 签名校验（明文模式） |
| 加密消息 | `callback.go` | AES-256-CBC 解密（加密模式） |
| Panic Recovery | `utils.go` | `recoverMiddleware` 防止崩溃 |
| 请求体限制 | `handlers.go` | `MaxBytesReader` 限制 1MB |
| Token 缓存隔离 | `wecom_api.go` | Redis/Memory 双模式，互不干扰 |
| 消息去重 | `callback.go` | MsgId 缓存 5 分钟，防止重复处理 |
| 操作确认 | `callback.go` | 危险操作（重启/删除）需二次确认 |

---

## 五、部署架构

```
┌─────────────────────────────────────────┐
│            Docker Host                   │
│                                          │
│  ┌────────────────────────────────────┐ │
│  │  wecom-nezha container             │ │
│  │  - 端口: 8080                      │ │
│  │  - 健康检查: /healthz              │ │
│  │  - 存储: /data (可选)              │ │
│  └────────────────────────────────────┘ │
│                                          │
│  ┌────────────────────────────────────┐ │
│  │  Redis container (可选)            │ │
│  │  - 端口: 6379                      │ │
│  │  - 用途: Token 缓存               │ │
│  └────────────────────────────────────┘ │
└─────────────────────────────────────────┘
         │                    │
         ▼                    ▼
  企业微信 API 服务器     哪吒监控面板
  (qyapi.weixin.qq.com)  (自建)
```

### 缓存模式选择

| 模式 | 适用场景 | 优缺点 |
|------|----------|--------|
| `none` | 开发/测试 | 每次请求都重新获取 Token，延迟高 |
| `memory` | 单实例部署 | 零依赖，重启后 Token 丢失需重新获取 |
| `redis` | 多实例/高可用 | 需要额外 Redis 服务，Token 持久化 |

---

## 六、扩展性设计

### 添加新的聊天命令

1. 在 `callback.go` 的 `processUserMessage()` 中添加命令匹配
2. 实现对应的处理函数
3. 如需调用哪吒 API，在 `nezha_api.go` 中添加封装函数
4. 更新帮助信息

### 添加新的推送渠道

1. 在 `handlers.go` 中添加新的处理器函数
2. 在 `main.go` 中注册路由
3. 在 `types.go` 中定义请求/响应结构体

### 添加新的外部 API 集成

1. 创建新的 `xxx_api.go` 文件
2. 在 `config.go` 中添加配置变量
3. 在 `docker-compose.yml` 中添加环境变量

# 哪吒监控（Nezha Monitoring）API 文档

> 文档版本: 1.0  
> 生成时间: 2026-04-10  
> 数据来源: `docs.go` / `swagger.json` / `swagger.yaml`

---

## 一、概述

哪吒监控是一个开源的服务器监控面板，支持服务器监控、服务监控、告警通知、DDNS、NAT 穿透、计划任务等功能。此文档为其后端 REST API 的完整接口说明。

**官方网站**: https://nezhahq.github.io  
**开源地址**: https://github.com/nezhahq/nezha

---

## 二、基本信息

| 项目 | 值 |
|------|-----|
| API 版本 | 1.0 |
| Base URL | `/api/v1` |
| 认证方式 | Bearer Token（API Key） |
| API Key 位置 | HTTP Header `Authorization` |
| 文档地址 | http://nezhahq.github.io |
| 联系邮箱 | hi@nai.ba |
| License | Apache 2.0 |

### 通用响应格式

所有接口均返回 JSON，结构如下：

```json
{
  "success": true,
  "error": "",
  "data": { ... }
}
```

- `success`: 请求是否成功
- `error`: 错误信息（成功时为空）
- `data`: 返回数据（类型因接口而异）

### 权限标签说明

| 标签 | 说明 |
|------|------|
| `auth required` | 需要登录认证 |
| `admin required` | 需要管理员权限 |
| `common` | 公开接口（部分需认证） |

---

## 三、接口列表

### 3.1 登录认证

#### POST `/login` — 用户登录

> **权限**: 无需认证

**请求参数**（Body）:
```json
{
  "username": "admin",
  "password": "your_password"
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
    "expire": "2026-04-11T00:00:00Z"
  }
}
```

---

#### GET `/refresh-token` — 刷新 Token

> **权限**: `auth required`

**响应**: 同登录接口

---

### 3.2 用户管理

#### GET `/user` — 用户列表

> **权限**: `admin required`

**响应**: `CommonResponse-array_model_User`

---

#### POST `/user` — 创建用户

> **权限**: `admin required`

**请求参数**（Body）: `model.UserForm`

---

### 3.3 个人资料

#### GET `/profile` — 获取个人资料

> **权限**: `auth required`

**响应**: `CommonResponse-model_Profile`

**返回字段**:
| 字段 | 类型 | 说明 |
|------|------|------|
| id | integer | 用户ID |
| username | string | 用户名 |
| role | integer | 角色 (0=管理员, 1=普通用户) |
| agent_secret | string | Agent 连接密钥 |
| login_ip | string | 上次登录IP |
| created_at | string | 创建时间 |
| oauth2_bind | object | OAuth2 绑定信息 |

---

#### POST `/profile` — 修改密码

> **权限**: `auth required`

**请求参数**（Body）: `model.ProfileForm`
```json
{
  "original_password": "旧密码",
  "new_password": "新密码"
}
```

---

### 3.4 服务器管理

#### GET `/server` — 服务器列表

> **权限**: `auth required`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| id | integer | 按资源ID筛选（可选） |

**响应**: `CommonResponse-array_model_Server`

**返回字段**:
| 字段 | 类型 | 说明 |
|------|------|------|
| id | integer | 服务器ID |
| name | string | 服务器名称 |
| uuid | string | 唯一标识 |
| host | object | 主机信息 |
| state | object | 实时状态 |
| geoip | object | 地理位置 |
| last_active | string | 最后活跃时间 |
| display_index | integer | 展示排序 |
| enable_ddns | boolean | 是否启用DDNS |
| public_note | string | 公开备注 |

**主机信息 (Host)**:
| 字段 | 类型 | 说明 |
|------|------|------|
| platform | string | 系统平台 |
| platform_version | string | 系统版本 |
| arch | string | CPU 架构 |
| cpu | array | CPU 型号列表 |
| mem_total | integer | 内存总量(字节) |
| disk_total | integer | 磁盘总量(字节) |
| gpu | array | GPU 列表 |
| boot_time | integer | 启动时间戳 |
| version | string | Agent 版本 |

**实时状态 (HostState)**:
| 字段 | 类型 | 说明 |
|------|------|------|
| cpu | number | CPU 使用率 |
| mem_used | integer | 已用内存(字节) |
| disk_used | integer | 已用磁盘(字节) |
| net_in_speed | integer | 入站网速(B/s) |
| net_out_speed | integer | 出站网速(B/s) |
| net_in_transfer | integer | 入站总流量(字节) |
| net_out_transfer | integer | 出站总流量(字节) |
| load_1/5/15 | number | 系统负载 |
| tcp_conn_count | integer | TCP 连接数 |
| udp_conn_count | integer | UDP 连接数 |
| temperatures | array | 温度传感器数据 |

---

#### PATCH `/server/{id}` — 编辑服务器

> **权限**: `auth required`

**请求参数**（Body）: `model.ServerForm`
```json
{
  "name": "新名称",
  "display_index": 100,
  "public_note": "备注信息"
}
```

---

### 3.5 服务器分组

#### GET `/server-group` — 服务器分组列表

> **权限**: `common`（列表公开，编辑需认证）

**响应**: `CommonResponse-array_model_ServerGroupResponseItem`

---

#### POST `/server-group` — 新建服务器分组

> **权限**: `auth required`

**请求参数**（Body）: `model.ServerGroupForm`
```json
{
  "name": "分组名称",
  "servers": [1, 2, 3]
}
```

---

#### PATCH `/server-group/{id}` — 编辑服务器分组

> **权限**: `auth required`

---

### 3.6 服务器监控数据

#### GET `/server/{id}/metrics` — 获取监控历史

> **权限**: `common`

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| metric | string | 是 | 指标名称 |
| period | string | 否 | 时间范围 (1d/7d/30d，默认1d) |

**支持的指标 (metric)**:
| 指标 | 说明 |
|------|------|
| cpu | CPU 使用率 |
| memory | 内存使用率 |
| swap | Swap 使用率 |
| disk | 磁盘使用率 |
| net_in_speed | 入站网速 |
| net_out_speed | 出站网速 |
| net_in_transfer | 入站总流量 |
| net_out_transfer | 出站总流量 |
| load1/5/15 | 系统负载 |
| tcp_conn | TCP 连接数 |
| udp_conn | UDP 连接数 |
| process_count | 进程数 |
| temperature | 温度 |
| uptime | 运行时间 |
| gpu | GPU 监控 |

**响应**: `CommonResponse-model_ServerMetricsResponse`
```json
{
  "success": true,
  "data": {
    "server_id": 1,
    "server_name": "Server-01",
    "metric": "cpu",
    "data_points": [
      { "ts": 1712707200, "value": 45.2 },
      { "ts": 1712707260, "value": 48.1 }
    ]
  }
}
```

---

### 3.7 服务监控

#### GET `/service` — 服务监控概览

> **权限**: `common`

**响应**: `CommonResponse-model_ServiceResponse`

返回所有服务监控的实时数据，包括每个服务的当前延迟、上下行流量等。

---

#### GET `/service/list` — 服务列表

> **权限**: `auth required`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| id | integer | 按ID筛选（可选） |

**响应**: `CommonResponse-array_model_Service`

---

#### POST `/service` — 创建服务监控

> **权限**: `auth required`

**请求参数**（Body）: `model.ServiceForm`
```json
{
  "name": "HTTP服务",
  "type": 1,
  "target": "https://example.com",
  "duration": 30,
  "notification_group_id": 1,
  "notify": true
}
```

**服务类型 (type)**:
| 值 | 说明 |
|---|------|
| 0 | TCP |
| 1 | HTTP |
| 2 | HTTPS |
| 3 | DNS |
| 4 | 端口检查 |
| 5 | ICMP Ping |

---

#### PATCH `/service/{id}` — 更新服务监控

> **权限**: `auth required`

---

#### GET `/service/{id}/history` — 服务监控历史

> **权限**: `common`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| period | string | 时间范围 (1d/7d/30d) |

---

#### GET `/service/server` — 有监控数据的服务器

> **权限**: `common`

返回部署了 Agent 且有服务监控数据的服务器ID列表。

---

#### GET `/server/{id}/service` — 指定服务器的服务历史

> **权限**: `common`

---

### 3.8 告警规则

#### GET `/alert-rule` — 告警规则列表

> **权限**: `auth required`

**响应**: `CommonResponse-array_model_AlertRule`

**返回字段**:
| 字段 | 类型 | 说明 |
|------|------|------|
| id | integer | 规则ID |
| name | string | 规则名称 |
| rules | array | 规则条件 |
| trigger_mode | integer | 触发模式 (0=始终, 1=单次) |
| enable | boolean | 是否启用 |
| notification_group_id | integer | 通知组ID |

**规则条件 (Rule)**:
| 字段 | 类型 | 说明 |
|------|------|------|
| type | string | 指标类型 |
| max/min | number | 阈值 |
| cover | integer | 覆盖范围 |
| cycle_interval | integer | 流量统计周期 |
| ignore | object | 排除的服务器 |

---

#### POST `/alert-rule` — 添加告警规则

> **权限**: `auth required`

**请求参数**（Body）: `model.AlertRuleForm`

---

#### PATCH `/alert-rule/{id}` — 更新告警规则

> **权限**: `auth required`

---

### 3.9 通知渠道

#### GET `/notification` — 通知列表

> **权限**: `auth required`

**响应**: `CommonResponse-array_model_Notification`

---

#### POST `/notification` — 添加通知渠道

> **权限**: `auth required`

**请求参数**（Body）: `model.NotificationForm`
```json
{
  "name": "钉钉通知",
  "url": "https://oapi.dingtalk.com/robot/send?access_token=xxx",
  "request_method": 1,
  "request_type": 1,
  "request_header": "Content-Type: application/json",
  "request_body": "{\"msgtype\":\"text\",\"text\":{\"content\":${msg}}}"
}
```

---

#### PATCH `/notification/{id}` — 编辑通知渠道

> **权限**: `auth required`

---

### 3.10 通知分组

#### GET `/notification-group` — 通知分组列表

> **权限**: `auth required`

---

#### POST `/notification-group` — 新建通知分组

> **权限**: `auth required`

**请求参数**（Body）: `model.NotificationGroupForm`
```json
{
  "name": "告警通知组",
  "notifications": [1, 2]
}
```

---

### 3.11 计划任务

#### GET `/cron` — 计划任务列表

> **权限**: `auth required`

**响应**: `CommonResponse-array_model_Cron`

---

#### POST `/cron` — 创建计划任务

> **权限**: `auth required`

**请求参数**（Body）: `model.CronForm`
```json
{
  "name": "备份任务",
  "command": "tar -czf backup.tar.gz /data",
  "scheduler": "0 2 * * *",
  "cover": 0,
  "servers": [1, 2, 3],
  "notification_group_id": 1,
  "push_successful": true,
  "task_type": 0
}
```

**参数说明**:
| 字段 | 类型 | 说明 |
|------|------|------|
| name | string | 任务名称 |
| command | string | 执行的命令 |
| scheduler | string | Cron 表达式 (分 时 日 月 星期) |
| cover | integer | 覆盖模式 (0=特定服务器, 1=忽略特定服务器, 2=触发服务器) |
| servers | array | 服务器ID列表 |
| notification_group_id | integer | 通知组ID |
| push_successful | boolean | 是否推送成功通知 |
| task_type | integer | 任务类型 (0=计划任务, 1=触发任务) |

---

#### PATCH `/cron/{id}` — 更新计划任务

> **权限**: `auth required`

---

#### GET `/cron/{id}/manual` — 手动触发任务

> **权限**: `auth required`

---

### 3.12 DDNS

#### GET `/ddns` — DDNS 配置列表

> **权限**: `auth required`

**响应**: `CommonResponse-array_model_DDNSProfile`

---

#### POST `/ddns` — 添加 DDNS 配置

> **权限**: `auth required`

**请求参数**（Body）: `model.DDNSForm`
```json
{
  "name": "我的DDNS",
  "provider": "aliyun",
  "access_id": "your_access_key",
  "access_secret": "your_secret",
  "domains": ["example.com"],
  "enable_ipv4": true,
  "enable_ipv6": false
}
```

---

#### PATCH `/ddns/{id}` — 编辑 DDNS 配置

> **权限**: `auth required`

---

#### GET `/ddns/providers` — DDNS 提供商列表

> **权限**: `auth required`

---

### 3.13 NAT 穿透

#### GET `/nat` — NAT 配置列表

> **权限**: `auth required`

**响应**: `CommonResponse-array_model_NAT`

---

#### POST `/nat` — 添加 NAT 配置

> **权限**: `auth required`

**请求参数**（Body）: `model.NATForm`
```json
{
  "name": "SSH穿透",
  "server_id": 1,
  "host": "内网IP",
  "domain": "子域名",
  "enabled": true
}
```

---

### 3.14 OAuth2 登录

#### GET `/oauth2/redirect` — 获取 OAuth2 重定向 URL

> **权限**: `auth required`

---

#### GET `/oauth2/callback` — OAuth2 回调

> **权限**: `无`

处理 OAuth2 提供商（如 GitHub、Google）的回调。

---

#### DELETE `/oauth2/unbind` — 解除 OAuth2 绑定

> **权限**: `auth required`

---

### 3.15 在线用户

#### GET `/online-user` — 在线用户列表

> **权限**: `auth required`

**请求参数**:
| 参数 | 类型 | 说明 |
|------|------|------|
| limit | integer | 每页数量 |
| offset | integer | 偏移量 |

---

#### POST `/online-user/batch-block` — 批量封禁在线用户

> **权限**: `admin required`

---

### 3.16 系统设置

#### GET `/setting` — 获取系统设置

> **权限**: `common`

**响应**: `CommonResponse-model_SettingResponse`

---

#### PATCH `/setting` — 修改系统设置

> **权限**: `admin required`

**请求参数**（Body）: `model.SettingForm`
```json
{
  "site_name": "我的监控",
  "language": "zh_CN",
  "dns_servers": "8.8.8.8,114.114.114.114"
}
```

---

### 3.17 系统维护

#### POST `/maintenance` — 执行系统维护

> **权限**: `admin required`

执行 SQLite VACUUM 和 TSDB 维护操作。

---

### 3.18 批量操作

所有批量删除接口请求体格式相同，为 ID 数组：

```json
[1, 2, 3, 4, 5]
```

| 接口 | 说明 |
|------|------|
| POST `/batch-delete/alert-rule` | 批量删除告警规则 |
| POST `/batch-delete/cron` | 批量删除计划任务 |
| POST `/batch-delete/ddns` | 批量删除 DDNS |
| POST `/batch-delete/nat` | 批量删除 NAT |
| POST `/batch-delete/notification` | 批量删除通知 |
| POST `/batch-delete/notification-group` | 批量删除通知分组 |
| POST `/batch-delete/server` | 批量删除服务器 |
| POST `/batch-delete/server-group` | 批量删除服务器分组 |
| POST `/batch-delete/service` | 批量删除服务监控 |
| POST `/batch-delete/user` | 批量删除用户 |
| POST `/batch-move/server` | 批量移动服务器到其他用户 |

---

### 3.19 Web SSH 终端

#### POST `/terminal` — 创建 Web SSH 会话

> **权限**: `auth required`

**请求参数**（Body）: `model.TerminalForm`
```json
{
  "id": 1,
  "cols": 80,
  "rows": 24
}
```

**响应**:
```json
{
  "success": true,
  "data": {
    "session_id": "uuid-xxx",
    "server_id": 1,
    "server_name": "Server-01"
  }
}
```

---

#### GET `/ws/terminal/{id}` — WebSocket 终端流

> **权限**: `auth required`

通过 WebSocket 连接进行终端交互。

---

### 3.20 文件管理器

#### GET `/file` — 创建文件管理器会话

> **权限**: `auth required`

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| id | integer | 是 | 服务器ID |

**响应**:
```json
{
  "success": true,
  "data": {
    "session_id": "uuid-xxx"
  }
}
```

---

#### GET `/ws/file/{id}` — WebSocket 文件流

> **权限**: `auth required`

---

### 3.21 Agent 强制更新

#### POST `/force-update/server` — 强制更新 Agent

> **权限**: `auth required`

**请求参数**（Body）:
```json
[1, 2, 3]
```

---

### 3.22 服务器配置

#### GET `/server/config/{id}` — 获取服务器配置

> **权限**: `auth required`

---

#### POST `/server/config` — 设置服务器配置

> **权限**: `auth required`

**请求参数**（Body）: `model.ServerConfigForm`
```json
{
  "config": "custom_config_value",
  "servers": [1, 2, 3]
}
```

---

### 3.23 WAF 防火墙

#### GET `/waf` — 封禁地址列表

> **权限**: `auth required`

---

#### PATCH `/waf` — 编辑封禁列表

> **权限**: `admin required`

---

## 四、使用示例

### 4.1 cURL 示例

**登录获取 Token**:
```bash
curl -X POST http://localhost:8008/api/v1/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your_password"}'
```

**获取服务器列表**:
```bash
curl http://localhost:8008/api/v1/server \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

**获取监控数据**:
```bash
curl "http://localhost:8008/api/v1/server/1/metrics?metric=cpu&period=1d" \
  -H "Authorization: Bearer YOUR_TOKEN_HERE"
```

### 4.2 Python 示例

```python
import requests

BASE_URL = "http://localhost:8008/api/v1"

# 登录
resp = requests.post(f"{BASE_URL}/login", json={
    "username": "admin",
    "password": "password"
})
token = resp.json()["data"]["token"]

headers = {"Authorization": f"Bearer {token}"}

# 获取服务器列表
servers = requests.get(f"{BASE_URL}/server", headers=headers).json()
print(f"共有 {len(servers['data'])} 台服务器")

# 获取 CPU 历史
metrics = requests.get(
    f"{BASE_URL}/server/1/metrics",
    params={"metric": "cpu", "period": "1d"},
    headers=headers
).json()
print(metrics)
```

### 4.3 JavaScript/Node.js 示例

```javascript
const axios = require('axios');

const API = 'http://localhost:8008/api/v1';

// 登录
const loginResp = await axios.post(`${API}/login`, {
  username: 'admin',
  password: 'password'
});
const token = loginResp.data.data.token;

// 获取服务器列表
const serversResp = await axios.get(`${API}/server`, {
  headers: { Authorization: `Bearer ${token}` }
});
console.log('服务器数量:', serversResp.data.data.length);

// 获取监控数据
const metricsResp = await axios.get(`${API}/server/1/metrics`, {
  params: { metric: 'memory', period: '7d' },
  headers: { Authorization: `Bearer ${token}` }
});
console.log(metricsResp.data);
```

---

## 五、数据模型参考

### CommonResponse 响应包装

所有响应都会被包装在以下格式中：

```json
{
  "success": true,
  "error": "",
  "data": {}
}
```

**data 类型对照表**:

| 接口 | data 类型 |
|------|-----------|
| GET /server | array_model_Server |
| GET /alert-rule | array_model_AlertRule |
| GET /service | model_ServiceResponse |
| GET /cron | array_model_Cron |
| POST /login | model_LoginResponse |
| GET /profile | model_Profile |
| GET /setting | model_SettingResponse |

---

## 六、注意事项

1. **认证**: 除 `/login` 外，所有接口都需要在 Header 中携带 `Authorization: Bearer <token>`
2. **权限**: 管理员接口需要 `Role = 0` 的用户 Token
3. **数据单位**: 流量单位为字节 (bytes)，速度单位为字节/秒 (B/s)
4. **时间格式**: 所有时间均为 ISO 8601 格式 (如 `2026-04-01T00:00:00Z`)
5. **监控指标**: GPU 和温度指标取决于服务器硬件和系统支持

---

*文档由 OpenClaw AI 助手自动生成*

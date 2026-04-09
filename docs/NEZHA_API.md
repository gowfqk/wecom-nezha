# Nezha V1 API 文档

> 基于哪吒监控 V1 版本源码整理

## 认证方式

- **Header**: `Authorization: Bearer <token>`
- 登录接口获取 token 后，在请求头中携带

## 服务器管理

### 获取服务器列表
- **GET** `/server`
- 认证: BearerAuth
- 参数: `id` (query, optional) - 服务器 ID
- 返回: `[]model.Server`

### 获取服务器详情
- **GET** `/server/{id}`
- 认证: BearerAuth

### 更新服务器
- **PATCH** `/server/{id}`
- 认证: BearerAuth
- Body: `model.ServerForm`

### 批量删除服务器
- **POST** `/batch-delete/server`
- 认证: BearerAuth
- Body: `[]uint64` (ID列表)

### 强制更新 Agent
- **POST** `/force-update/server`
- 认证: BearerAuth
- Body: `[]uint64` (服务器ID列表)

### 获取服务器配置
- **GET** `/server/config/{id}`
- 认证: BearerAuth

### 添加服务器配置
- **POST** `/server/config`
- 认证: BearerAuth

### 获取服务器指标
- **GET** `/server/{id}/metrics`
- 认证: 无需认证 (common)

### 移动服务器到分组
- **POST** `/batch-move/server`
- 认证: BearerAuth

---

## 服务管理

### 获取服务列表
- **GET** `/service`
- 认证: BearerAuth (部分)

### 添加服务
- **POST** `/service`
- 认证: BearerAuth

### 更新服务
- **PATCH** `/service/{id}`
- 认证: BearerAuth

### 删除服务
- **POST** `/batch-delete/service`
- 认证: BearerAuth

### 获取服务列表（简化）
- **GET** `/service/list`
- 认证: common

### 获取服务器关联服务
- **GET** `/service/server`
- 认证: common

### 获取服务历史
- **GET** `/service/{id}/history`
- 认证: common

---

## 用户管理

### 用户列表
- **GET** `/user`
- 认证: admin required

### 添加用户
- **POST** `/user`
- 认证: admin required

### 批量删除用户
- **POST** `/batch-delete/user`
- 认证: admin required

### 获取当前用户资料
- **GET** `/profile`
- 认证: auth required

### 更新当前用户资料
- **POST** `/profile`
- 认证: auth required

### 在线用户列表
- **GET** `/online-user`
- 认证: auth required

### 踢出在线用户
- **POST** `/online-user/batch-block`
- 认证: admin required

---

## 通知管理

### 通知列表
- **GET** `/notification`
- 认证: auth required

### 添加通知
- **POST** `/notification`
- 认证: auth required

### 更新通知
- **PATCH** `/notification/{id}`
- 认证: auth required

### 批量删除通知
- **POST** `/batch-delete/notification`
- 认证: auth required

### 通知组
- **GET/POST** `/notification-group`
- **PATCH** `/notification-group/{id}`
- 认证: auth required

---

## 定时任务

### 任务列表
- **GET** `/cron`
- 认证: auth required

### 添加任务
- **POST** `/cron`
- 认证: auth required

### 更新任务
- **PATCH** `/cron/{id}`
- 认证: auth required

### 手动执行任务
- **GET** `/cron/{id}/manual`
- 认证: auth required

### 批量删除任务
- **POST** `/batch-delete/cron`
- 认证: auth required

---

## DDNS

### DDNS 列表
- **GET** `/ddns`
- 认证: auth required

### 添加 DDNS
- **POST** `/ddns`
- 认证: auth required

### 更新 DDNS
- **PATCH** `/ddns/{id}`
- 认证: auth required

### DDNS 提供商列表
- **GET** `/ddns/providers`
- 认证: auth required

### 批量删除 DDNS
- **POST** `/batch-delete/ddns`
- 认证: auth required

---

## NAT 内网穿透

### NAT 列表
- **GET** `/nat`
- 认证: auth required

### 添加 NAT
- **POST** `/nat`
- 认证: auth required

### 更新 NAT
- **PATCH** `/nat/{id}`
- 认证: auth required

### 批量删除 NAT
- **POST** `/batch-delete/nat`
- 认证: auth required

---

## 系统设置

### 获取设置
- **GET** `/setting`
- 认证: common

### 更新设置
- **PATCH** `/setting`
- 认证: admin required

### 执行维护操作
- **POST** `/maintenance`
- 认证: admin required

---

## WAF

### WAF 列表
- **GET** `/waf`
- 认证: auth required

### 更新 WAF
- **PATCH** `/batch-delete/waf`
- 认证: admin required

---

## 文件管理

### 文件列表
- **GET** `/file`
- 认证: auth required

### 下载文件
- **GET** `/ws/file/{id}`
- 认证: auth required

---

## 服务器分组

### 分组列表
- **GET** `/server-group`
- 认证: auth required

### 分组操作
- **POST/PATCH** `/server-group/{id}`
- 认证: auth required

### 批量删除分组
- **POST** `/batch-delete/server-group`
- 认证: auth required

---

## 告警规则

### 告警规则列表
- **GET** `/alert-rule`
- 认证: auth required

### 告警规则操作
- **POST/PATCH** `/alert-rule/{id}`
- 认证: auth required

### 批量删除告警规则
- **POST** `/batch-delete/alert-rule`
- 认证: auth required

---

## 登录认证

### 登录
- **POST** `/login`
- Body: 用户名密码

### 刷新 Token
- **POST** `/refresh-token`

### OAuth 登录
- **GET** `/api/v1/oauth2/{provider}`
- **GET** `/api/v1/oauth2/callback`

---

## 可实现的功能

| 功能 | 说明 |
|------|------|
| 服务器列表/详情 | 获取所有服务器及状态 |
| 服务器监控 | CPU/内存/磁盘/网络实时数据 |
| 离线检测 | 通过 last_active 判断服务器是否在线 |
| 服务管理 | 添加/修改/删除监控服务 |
| 告警通知 | 通过通知 API 实现自定义告警 |
| 用户管理 | 增删改查用户 |
| 定时任务 | 创建和管理定时任务 |
| DDNS | 动态域名解析 |
| NAT 穿透 | 内网穿透配置 |
| 系统维护 | 触发面板维护操作 |


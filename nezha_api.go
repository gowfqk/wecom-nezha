package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

var nezhaAccessToken = ""
var nezhaTokenExpire time.Time

// 服务器列表短期缓存（减少重复 API 调用）
var serverListCache struct {
	sync.RWMutex
	servers    []NezhaServer
	expireTime time.Time
}

const serverListCacheTTL = 10 * time.Second

func nezhaRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if nezhaAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+nezhaAccessToken)
	}
	return httpClient.Do(req)
}

// NezhaLogin 登录获取 Token
func NezhaLogin() error {
	if NezhaUrl == "" || NezhaUsername == "" || NezhaPassword == "" {
		return fmt.Errorf("Nezha登录配置未设置")
	}

	// 检查 token 是否还有效（提前5分钟过期）
	if nezhaAccessToken != "" && time.Now().Add(5*time.Minute).Before(nezhaTokenExpire) {
		return nil
	}

	// token 即将过期或已过期，优先尝试 refresh-token（无需重新输入密码）
	if nezhaAccessToken != "" {
		if err := RefreshNezhaToken(); err == nil {
			logger.Println("Nezha Token 刷新成功")
			return nil
		} else {
			logger.Printf("Nezha Token 刷新失败，回退到重新登录: %v", err)
		}
	}

	// 刷新失败或无旧 token，执行完整登录
	return nezhaFullLogin()
}

// RefreshNezhaToken 使用已有 token 调用 /refresh-token 续期
func RefreshNezhaToken() error {
	url := fmt.Sprintf("%s/api/v1/refresh-token", strings.TrimRight(NezhaUrl, "/"))

	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("刷新请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取刷新响应失败: %v", err)
	}

	if resp.StatusCode != 200 {
		return fmt.Errorf("刷新失败（HTTP %d）", resp.StatusCode)
	}

	var result NezhaLoginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析刷新响应失败: %v", err)
	}

	if !result.Success {
		return fmt.Errorf("刷新失败: %s", result.Error)
	}

	nezhaAccessToken = result.Data.Token
	expireTime, err := time.Parse(time.RFC3339, result.Data.Expire)
	if err != nil {
		nezhaTokenExpire = time.Now().Add(time.Hour)
	} else {
		nezhaTokenExpire = expireTime
	}

	return nil
}

// nezhaFullLogin 使用用户名密码完整登录
func nezhaFullLogin() error {
	url := fmt.Sprintf("%s/api/v1/login", strings.TrimRight(NezhaUrl, "/"))
	loginData := map[string]string{
		"username": NezhaUsername,
		"password": NezhaPassword,
	}

	jsonData, err := json.Marshal(loginData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode != 200 {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return fmt.Errorf("登录失败（HTTP %d）: %s", resp.StatusCode, raw)
	}

	var result NezhaLoginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return fmt.Errorf("登录响应解析失败: %s", raw)
	}

	if !result.Success {
		return fmt.Errorf("登录失败: %s", result.Error)
	}

	nezhaAccessToken = result.Data.Token
	// 解析过期时间
	expireTime, err := time.Parse(time.RFC3339, result.Data.Expire)
	if err != nil {
		// 如果解析失败，默认1小时后过期
		nezhaTokenExpire = time.Now().Add(time.Hour)
	} else {
		nezhaTokenExpire = expireTime
	}

	return nil
}

// GetNezhaServerList 获取服务器列表（带短期缓存）
func GetNezhaServerList() ([]NezhaServer, error) {
	// 检查缓存
	serverListCache.RLock()
	if !serverListCache.expireTime.IsZero() && time.Now().Before(serverListCache.expireTime) {
		servers := serverListCache.servers
		serverListCache.RUnlock()
		return servers, nil
	}
	serverListCache.RUnlock()

	// 缓存过期，重新获取
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/server", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return nil, fmt.Errorf("API请求失败（HTTP %d）: %s", resp.StatusCode, raw)
	}

	var result NezhaAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return nil, fmt.Errorf("JSON解析失败: %s", raw)
	}

	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	data, err := json.Marshal(result.Data)
	if err != nil {
		return nil, err
	}

	var servers []NezhaServer
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, err
	}

	// 补充 ValidIP + 校验在线状态
	now := time.Now().Unix()
	for i := range servers {
		// API 返回的 online 字段优先，仅在 last_active 有效时用它校验
		if int64(servers[i].LastActive) > 0 {
			servers[i].Online = now-int64(servers[i].LastActive) < 300 // 5分钟内活跃视为在线
		}
		// API 不返回 valid_ip，从 name 中提取（格式如 "ecs(10.0.0.1)"）
		if servers[i].ValidIP == "" {
			if ip := extractIPFromName(servers[i].Name); ip != "" {
				servers[i].ValidIP = ip
			}
		}
	}

	// 写入缓存
	serverListCache.Lock()
	serverListCache.servers = servers
	serverListCache.expireTime = time.Now().Add(serverListCacheTTL)
	serverListCache.Unlock()

	return servers, nil
}

// FindServerResult 服务器查找结果
type FindServerResult struct {
	Server  *NezhaServer   // 唯一匹配时返回
	Matched []NezhaServer  // 多个匹配时返回列表
}

// FindServer 统一的服务器查找函数（精确匹配 + 模糊匹配）
// matchTag: 是否同时匹配 Note 和 Tag 字段
func FindServer(name string, matchTag bool) (*FindServerResult, error) {
	servers, err := GetNezhaServerList()
	if err != nil {
		return nil, err
	}

	// 精确匹配
	for i := range servers {
		if servers[i].Name == name {
			return &FindServerResult{Server: &servers[i]}, nil
		}
	}

	// 模糊匹配
	lowerName := strings.ToLower(name)
	var matched []NezhaServer
	for _, s := range servers {
		if strings.Contains(strings.ToLower(s.Name), lowerName) {
			matched = append(matched, s)
		} else if matchTag {
			if strings.Contains(strings.ToLower(s.Note), lowerName) ||
				strings.Contains(strings.ToLower(s.Tag), lowerName) {
				matched = append(matched, s)
			}
		}
	}

	if len(matched) == 0 {
		return &FindServerResult{}, nil
	}
	if len(matched) == 1 {
		return &FindServerResult{Server: &matched[0]}, nil
	}
	return &FindServerResult{Matched: matched}, nil
}

// GetNezhaServerByName 根据名称精确获取服务器
func GetNezhaServerByName(name string) (*NezhaServer, error) {
	result, err := FindServer(name, false)
	if err != nil {
		return nil, err
	}
	if result.Server != nil {
		return result.Server, nil
	}
	return nil, fmt.Errorf("未找到服务器: %s", name)
}

// GetOfflineServers 获取离线服务器列表
func GetOfflineServers() ([]NezhaServer, error) {
	servers, err := GetNezhaServerList()
	if err != nil {
		return nil, err
	}

	var offline []NezhaServer
	for _, s := range servers {
		if !s.Online {
			offline = append(offline, s)
		}
	}

	return offline, nil
}

// GetAgentSecret 获取 Agent Secret
func GetAgentSecret() (string, error) {
	if err := NezhaLogin(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/v1/profile", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			AgentSecret string `json:"agent_secret"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("解析profile失败: %v", err)
	}
	if !result.Success {
		return "", fmt.Errorf("获取profile失败")
	}
	if result.Data.AgentSecret == "" {
		return "", fmt.Errorf("Agent Secret 为空")
	}
	return result.Data.AgentSecret, nil
}

func boolToTLS(url string) string {
	if strings.HasPrefix(url, "https") {
		return "true"
	}
	return "false"
}

// RebootNezhaServer 通过创建触发任务重启服务器
func RebootNezhaServer(serverID uint, platform string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	command := "reboot"
	if strings.Contains(strings.ToLower(platform), "windows") {
		command = "shutdown /r /t 0"
	}

	url := fmt.Sprintf("%s/api/v1/cron", strings.TrimRight(NezhaUrl, "/"))
	taskData := map[string]interface{}{
		"name":      "手动重启",
		"command":   command,
		"scheduler": "@every 1m",
		"cover":     0,
		"servers":   []uint{serverID},
		"task_type": 1, // 触发任务
	}

	jsonData, err := json.Marshal(taskData)
	if err != nil {
		return err
	}

	logger.Printf("重启: 创建触发任务, serverID=%d", serverID)
	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("创建任务请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	logger.Printf("重启: 创建任务响应 (HTTP %d): %s", resp.StatusCode, string(body))

	var taskResult struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    uint   `json:"data"`
	}
	if err := json.Unmarshal(body, &taskResult); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}
	if !taskResult.Success {
		return fmt.Errorf("创建任务失败: %s", taskResult.Error)
	}

	if taskResult.Data == 0 {
		return fmt.Errorf("创建任务成功但未返回任务ID")
	}

	// 手动触发任务
	triggerURL := fmt.Sprintf("%s/api/v1/cron/%d/manual", strings.TrimRight(NezhaUrl, "/"), taskResult.Data)
	logger.Printf("重启: 触发任务, taskID=%d, url=%s", taskResult.Data, triggerURL)
	triggerResp, err := nezhaRequest("GET", triggerURL, nil)
	if err != nil {
		return fmt.Errorf("触发任务失败: %v", err)
	}
	defer triggerResp.Body.Close()

	triggerBody, _ := io.ReadAll(triggerResp.Body)
	logger.Printf("重启: 触发响应 (HTTP %d): %s", triggerResp.StatusCode, string(triggerBody))

	if triggerResp.StatusCode != 200 {
		return fmt.Errorf("触发任务失败 (HTTP %d): %s", triggerResp.StatusCode, string(triggerBody))
	}

	return nil
}

// GetServerByID 获取单个服务器完整信息
func GetServerByID(serverID uint) (map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/server?id=%d", strings.TrimRight(NezhaUrl, "/"), serverID)
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("未找到服务器 ID=%d", serverID)
	}

	return result.Data[0], nil
}

// UpdateServerField 通用服务器字段更新
// field: "name"(显示名称), "note"(私有标签), "public_note"(备注)
func UpdateServerField(serverID uint, field, value string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	// 先读取当前服务器信息
	server, err := GetServerByID(serverID)
	if err != nil {
		return err
	}

	// 修改目标字段
	server[field] = value

	// 清理不需要提交的字段
	delete(server, "id")
	delete(server, "created_at")
	delete(server, "updated_at")
	delete(server, "last_active")
	delete(server, "state")
	delete(server, "host")
	delete(server, "geoip")

	url := fmt.Sprintf("%s/api/v1/server/%d", strings.TrimRight(NezhaUrl, "/"), serverID)
	jsonData, _ := json.Marshal(server)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新失败: %s", result.Error)
	}

	return nil
}

// UpdateServerNote 更新服务器备注（保留原有字段）
func UpdateServerNote(serverID uint, publicNote string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	// 先读取当前服务器信息
	server, err := GetServerByID(serverID)
	if err != nil {
		return err
	}

	// 只修改 public_note，保留其他字段
	server["public_note"] = publicNote

	// 清理不需要提交的字段
	delete(server, "id")
	delete(server, "created_at")
	delete(server, "updated_at")
	delete(server, "last_active")
	delete(server, "state")
	delete(server, "host")
	delete(server, "geoip")

	url := fmt.Sprintf("%s/api/v1/server/%d", strings.TrimRight(NezhaUrl, "/"), serverID)
	jsonData, _ := json.Marshal(server)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新备注失败: %s", result.Error)
	}

	return nil
}

// UpdateServerTag 更新服务器私有备注/标签（保留原有字段）
func UpdateServerTag(serverID uint, tag string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	// 先读取当前服务器信息
	server, err := GetServerByID(serverID)
	if err != nil {
		return err
	}

	// 只修改 note 字段（私有备注/标签），保留其他字段
	server["note"] = tag

	// 清理不需要提交的字段
	delete(server, "id")
	delete(server, "created_at")
	delete(server, "updated_at")
	delete(server, "last_active")
	delete(server, "state")
	delete(server, "host")
	delete(server, "geoip")

	url := fmt.Sprintf("%s/api/v1/server/%d", strings.TrimRight(NezhaUrl, "/"), serverID)
	jsonData, _ := json.Marshal(server)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新标签失败: %s", result.Error)
	}

	return nil
}

func GetNatList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/nat", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddNat 添加 NAT 穿透配置
func AddNat(name, domain, host string, serverID uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/nat", strings.TrimRight(NezhaUrl, "/"))
	natData := map[string]interface{}{
		"name":      name,
		"domain":    domain,
		"host":      host,
		"server_id": serverID,
		"enabled":   true,
	}

	jsonData, err := json.Marshal(natData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加NAT失败: %s", result.Error)
	}

	return nil
}

// DeleteNat 删除 NAT 配置
func DeleteNat(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/nat", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除NAT失败: %s", result.Error)
	}

	return nil
}

// UpdateNat 更新 NAT 配置（内网地址和/或服务器）
// serverID 为 0 表示不修改服务器
func UpdateNat(id uint, host string, serverID uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/nat/%d", strings.TrimRight(NezhaUrl, "/"), id)
	updateData := map[string]interface{}{}
	if host != "" {
		updateData["host"] = host
	}
	if serverID > 0 {
		updateData["server_id"] = serverID
	}
	jsonData, _ := json.Marshal(updateData)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新NAT失败: %s", result.Error)
	}

	return nil
}

// ToggleNat 启用/禁用 NAT 配置
func ToggleNat(id uint, enabled bool) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/nat/%d", strings.TrimRight(NezhaUrl, "/"), id)
	jsonData, _ := json.Marshal(map[string]interface{}{
		"enabled": enabled,
	})

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("操作失败: %s", result.Error)
	}

	return nil
}


// MetricsDataPoint 监控数据点
// 兼容两种 API 响应格式：
//   格式1（嵌套）: {"data": {"data_points": [{"ts": 123, "value": 45.2}]}}
//   格式2（扁平）: {"data": [{"created_at": 123, "avg_val": 45.2}]}
type MetricsDataPoint struct {
	Timestamp int64   // 时间戳
	Avg       float64 // 数值
}

func (m *MetricsDataPoint) UnmarshalJSON(data []byte) error {
	// 尝试多种字段名组合
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// 时间戳：ts > created_at > timestamp
	if v, ok := raw["ts"]; ok {
		m.Timestamp = toInt64(v)
	} else if v, ok := raw["created_at"]; ok {
		m.Timestamp = toInt64(v)
	} else if v, ok := raw["timestamp"]; ok {
		m.Timestamp = toInt64(v)
	}

	// 数值：value > avg_val > avg
	if v, ok := raw["value"]; ok {
		m.Avg = toFloat64(v)
	} else if v, ok := raw["avg_val"]; ok {
		m.Avg = toFloat64(v)
	} else if v, ok := raw["avg"]; ok {
		m.Avg = toFloat64(v)
	}

	return nil
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case float64:
		return int64(val)
	case int64:
		return val
	case json.Number:
		n, _ := val.Int64()
		return n
	}
	return 0
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case json.Number:
		n, _ := val.Float64()
		return n
	}
	return 0
}

// GetServerMetrics 获取服务器监控历史数据
func GetServerMetrics(serverID uint, metric string, period string) ([]MetricsDataPoint, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	if period == "" {
		period = "1d"
	}

	url := fmt.Sprintf("%s/api/v1/server/%d/metrics?metric=%s&period=%s",
		strings.TrimRight(NezhaUrl, "/"), serverID, metric, period)

	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return nil, fmt.Errorf("API请求失败（HTTP %d）: %s", resp.StatusCode, raw)
	}

	// 先尝试嵌套格式（官方文档格式）:
	// {"success":true,"data":{"data_points":[{"ts":123,"value":45.2}]}}
	var nestedResult struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
		Data    struct {
			DataPoints []MetricsDataPoint `json:"data_points"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &nestedResult); err == nil && nestedResult.Success && len(nestedResult.Data.DataPoints) > 0 {
		return nestedResult.Data.DataPoints, nil
	}

	// 再尝试扁平格式:
	// {"success":true,"data":[{"created_at":123,"avg_val":45.2}]}
	var flatResult struct {
		Success bool               `json:"success"`
		Error   string             `json:"error"`
		Data    []MetricsDataPoint `json:"data"`
	}
	if err := json.Unmarshal(body, &flatResult); err != nil {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return nil, fmt.Errorf("解析监控数据失败: %s", raw)
	}

	if !flatResult.Success {
		return nil, fmt.Errorf("获取监控数据失败: %s", flatResult.Error)
	}

	return flatResult.Data, nil
}

// ServiceInfo 服务监控信息
type ServiceInfo struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Type     uint   `json:"type"`
	Target   string `json:"target"`
	Duration uint   `json:"duration"`
}

// ServiceStatus 服务监控状态（从 /service 接口解析）
type ServiceStatus struct {
	Name      string
	Target    string
	Type      uint
	AvgDelay  float64
	Online    bool
}

// GetServiceList 获取服务监控状态
func GetServiceList() ([]ServiceStatus, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/service/list", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		raw := string(body)
		if len(raw) > 200 {
			raw = raw[:200]
		}
		return nil, fmt.Errorf("API请求失败（HTTP %d）: %s", resp.StatusCode, raw)
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("解析服务数据失败: %v", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("获取服务列表失败: %s", result.Error)
	}

	var services []ServiceStatus
	for _, s := range result.Data {
		name, _ := s["name"].(string)
		target, _ := s["target"].(string)
		sType := uint(0)
		if t, ok := s["type"].(float64); ok {
			sType = uint(t)
		}
		avgDelay := float64(0)
		if d, ok := s["avg_delay"].(float64); ok {
			avgDelay = d
		}
		// 判断在线状态：有延迟数据视为在线
		online := avgDelay > 0

		services = append(services, ServiceStatus{
			Name:     name,
			Target:   target,
			Type:     sType,
			AvgDelay: avgDelay,
			Online:   online,
		})
	}

	return services, nil
}


// ========== DDNS 管理 ==========

// GetDDNSList 获取 DDNS 配置列表
func GetDDNSList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/ddns", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// GetDDNSProviders 获取 DDNS 提供商列表
func GetDDNSProviders() ([]string, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/ddns/providers", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool     `json:"success"`
		Error   string   `json:"error"`
		Data    []string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddDDNS 添加 DDNS 配置
func AddDDNS(name, provider, accessID, accessSecret string, domains []string, enableIPv4, enableIPv6 bool) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/ddns", strings.TrimRight(NezhaUrl, "/"))
	ddnsData := map[string]interface{}{
		"name":          name,
		"provider":      provider,
		"access_id":     accessID,
		"access_secret": accessSecret,
		"domains":       domains,
		"enable_ipv4":   enableIPv4,
		"enable_ipv6":   enableIPv6,
	}

	jsonData, err := json.Marshal(ddnsData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加DDNS失败: %s", result.Error)
	}

	return nil
}

// DeleteDDNS 删除 DDNS 配置
func DeleteDDNS(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/ddns", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除DDNS失败: %s", result.Error)
	}

	return nil
}

// UpdateDDNS 更新 DDNS 配置
func UpdateDDNS(id uint, updateData map[string]interface{}) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/ddns/%d", strings.TrimRight(NezhaUrl, "/"), id)
	jsonData, _ := json.Marshal(updateData)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新DDNS失败: %s", result.Error)
	}

	return nil
}

// ========== 通知渠道管理 ==========

// GetNotificationList 获取通知渠道列表
func GetNotificationList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/notification", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddNotification 添加通知渠道
func AddNotification(name, notifyURL string, requestMethod, requestType uint, requestHeader, requestBody string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/notification", strings.TrimRight(NezhaUrl, "/"))
	notifyData := map[string]interface{}{
		"name":           name,
		"url":            notifyURL,
		"request_method": requestMethod,
		"request_type":   requestType,
		"request_header": requestHeader,
		"request_body":   requestBody,
	}

	jsonData, err := json.Marshal(notifyData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加通知渠道失败: %s", result.Error)
	}

	return nil
}

// DeleteNotification 删除通知渠道
func DeleteNotification(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/notification", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除通知渠道失败: %s", result.Error)
	}

	return nil
}

// UpdateNotification 更新通知渠道
func UpdateNotification(id uint, updateData map[string]interface{}) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/notification/%d", strings.TrimRight(NezhaUrl, "/"), id)
	jsonData, _ := json.Marshal(updateData)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新通知渠道失败: %s", result.Error)
	}

	return nil
}

// ========== 告警规则管理 ==========

// GetAlertRuleList 获取告警规则列表
func GetAlertRuleList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/alert-rule", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddAlertRule 添加告警规则
func AddAlertRule(ruleData map[string]interface{}) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/alert-rule", strings.TrimRight(NezhaUrl, "/"))
	jsonData, err := json.Marshal(ruleData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加告警规则失败: %s", result.Error)
	}

	return nil
}

// DeleteAlertRule 删除告警规则
func DeleteAlertRule(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/alert-rule", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除告警规则失败: %s", result.Error)
	}

	return nil
}

// ========== 定时任务管理 ==========

// GetCronList 获取定时任务列表
func GetCronList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/cron", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddCron 添加定时任务
func AddCron(cronData map[string]interface{}) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/cron", strings.TrimRight(NezhaUrl, "/"))
	jsonData, err := json.Marshal(cronData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加定时任务失败: %s", result.Error)
	}

	return nil
}

// DeleteCron 删除定时任务
func DeleteCron(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/cron", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除定时任务失败: %s", result.Error)
	}

	return nil
}

// TriggerCron 手动触发定时任务
func TriggerCron(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/cron/%d/manual", strings.TrimRight(NezhaUrl, "/"), id)
	resp, err := nezhaRequest("POST", url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("触发任务失败: %s", result.Error)
	}

	return nil
}

// ========== 服务监控管理 ==========

// GetServiceDetail 获取单个服务监控详情
func GetServiceDetail(id uint) (map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/service?id=%d", strings.TrimRight(NezhaUrl, "/"), id)
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                   `json:"success"`
		Error   string                 `json:"error"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// GetServiceHistory 获取服务监控历史
func GetServiceHistory(id uint) ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/service/%d/history", strings.TrimRight(NezhaUrl, "/"), id)
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// GetServerServices 获取指定服务器的服务监控列表
func GetServerServices(serverID uint) ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/server/%d/service", strings.TrimRight(NezhaUrl, "/"), serverID)
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddService 添加服务监控
func AddService(serviceData map[string]interface{}) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/service", strings.TrimRight(NezhaUrl, "/"))
	jsonData, err := json.Marshal(serviceData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加服务监控失败: %s", result.Error)
	}

	return nil
}

// DeleteService 删除服务监控
func DeleteService(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/service", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除服务监控失败: %s", result.Error)
	}

	return nil
}

// ========== 通知分组管理 ==========

// GetNotificationGroupList 获取通知分组列表
func GetNotificationGroupList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/notification-group", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// AddNotificationGroup 添加通知分组
func AddNotificationGroup(groupData map[string]interface{}) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/notification-group", strings.TrimRight(NezhaUrl, "/"))
	jsonData, err := json.Marshal(groupData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("添加通知分组失败: %s", result.Error)
	}

	return nil
}

// DeleteNotificationGroup 删除通知分组
func DeleteNotificationGroup(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/notification-group", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除通知分组失败: %s", result.Error)
	}

	return nil
}

// ========== 服务器管理 ==========

// GetServerGroupList 获取服务器分组列表
func GetServerGroupList() ([]map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/server-group", strings.TrimRight(NezhaUrl, "/"))
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                     `json:"success"`
		Error   string                   `json:"error"`
		Data    []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// GetServerGroup 获取单个服务器分组详情
func GetServerGroup(id uint) (map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/server-group/%d", strings.TrimRight(NezhaUrl, "/"), id)
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                   `json:"success"`
		Error   string                 `json:"error"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// GetServersInGroup 获取分组下的服务器列表
func GetServersInGroup(groupID uint) ([]NezhaServer, error) {
	servers, err := GetNezhaServerList()
	if err != nil {
		return nil, err
	}

	var result []NezhaServer
	for _, s := range servers {
		if s.GroupID == groupID {
			result = append(result, s)
		}
	}
	return result, nil
}

// GetNotifyGroup 获取单个通知分组详情
func GetNotifyGroup(id uint) (map[string]interface{}, error) {
	if err := NezhaLogin(); err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/v1/notification-group/%d", strings.TrimRight(NezhaUrl, "/"), id)
	resp, err := nezhaRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Success bool                   `json:"success"`
		Error   string                 `json:"error"`
		Data    map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %s", string(body))
	}
	if !result.Success {
		return nil, fmt.Errorf("API错误: %s", result.Error)
	}

	return result.Data, nil
}

// CreateServerGroup 创建服务器分组
func CreateServerGroup(name string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/server-group", strings.TrimRight(NezhaUrl, "/"))
	groupData := map[string]interface{}{
		"name": name,
	}
	jsonData, err := json.Marshal(groupData)
	if err != nil {
		return err
	}

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("创建分组失败: %s", result.Error)
	}

	return nil
}

// DeleteServerGroup 删除服务器分组
func DeleteServerGroup(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/server-group", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除分组失败: %s", result.Error)
	}

	return nil
}

// UpdateServerGroup 更新服务器分组
func UpdateServerGroup(id uint, name string) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/server-group/%d", strings.TrimRight(NezhaUrl, "/"), id)
	groupData := map[string]interface{}{
		"name": name,
	}
	jsonData, _ := json.Marshal(groupData)

	resp, err := nezhaRequest("PATCH", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("更新分组失败: %s", result.Error)
	}

	return nil
}

// DeleteServer 删除服务器
func DeleteServer(id uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/batch-delete/server", strings.TrimRight(NezhaUrl, "/"))
	jsonData, _ := json.Marshal([]uint{id})

	resp, err := nezhaRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %s", string(body))
	}
	if !result.Success {
		return fmt.Errorf("删除服务器失败: %s", result.Error)
	}

	return nil
}

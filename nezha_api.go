package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var nezhaAccessToken = ""
var nezhaTokenExpire time.Time

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

// GetNezhaServerList 获取服务器列表
func GetNezhaServerList() ([]NezhaServer, error) {
	// 先登录获取 token
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

	// 计算在线状态 + 补充 ValidIP
	now := time.Now().Unix()
	for i := range servers {
		servers[i].Online = now-int64(servers[i].LastActive) < 300 // 5分钟内活跃视为在线
		// API 不返回 valid_ip，从 name 中提取（格式如 "ecs(10.0.0.1)"）
		if servers[i].ValidIP == "" {
			if ip := extractIPFromName(servers[i].Name); ip != "" {
				servers[i].ValidIP = ip
			}
		}
	}

	return servers, nil
}

// GetNezhaServerByName 根据名称获取服务器
func GetNezhaServerByName(name string) (*NezhaServer, error) {
	servers, err := GetNezhaServerList()
	if err != nil {
		return nil, err
	}

	for _, s := range servers {
		if s.Name == name {
			return &s, nil
		}
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

// GetNatList 获取 NAT 穿透列表
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

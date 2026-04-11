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
	if NezhaBasicAuthUser != "" {
		req.SetBasicAuth(NezhaBasicAuthUser, NezhaBasicAuthPass)
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
func RebootNezhaServer(serverID uint) error {
	if err := NezhaLogin(); err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/v1/cron", strings.TrimRight(NezhaUrl, "/"))
	taskData := map[string]interface{}{
		"name":        "手动重启",
		"command":     "reboot",
		"scheduler":   "@every 1m",
		"cover":       0,
		"servers":     []uint{serverID},
		"task_type":   1, // 触发任务
	}

	jsonData, err := json.Marshal(taskData)
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

	var result struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}
	if !result.Success {
		return fmt.Errorf("API错误: %s", result.Error)
	}

	// 获取刚创建的任务ID并手动触发
	var taskResult struct {
		Success bool `json:"success"`
		Data    struct {
			ID uint `json:"id"`
		} `json:"data"`
	}
	json.Unmarshal(body, &taskResult)
	if taskResult.Data.ID > 0 {
		triggerURL := fmt.Sprintf("%s/api/v1/cron/%d/manual", strings.TrimRight(NezhaUrl, "/"), taskResult.Data.ID)
		nezhaRequest("GET", triggerURL, nil) // 忽略结果，触发即可
	}

	return nil
}

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

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result NezhaLoginResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return err
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+nezhaAccessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result NezhaAPIResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
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

// GetAgentInstallCommand 获取 Agent 安装命令
func GetAgentInstallCommand() (string, error) {
	if err := NezhaLogin(); err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/api/v1/profile", strings.TrimRight(NezhaUrl, "/"))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+nezhaAccessToken)

	resp, err := httpClient.Do(req)
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

	cmd := fmt.Sprintf("curl -L https://raw.githubusercontent.com/nezhahq/scripts/main/install.sh -o nezha.sh && chmod +x nezha.sh && env NZ_SERVER=\"%s\" NZ_TLS=\"%s\" NZ_CLIENT_SECRET=\"%s\" ./nezha.sh",
		NezhaUrl, boolToTLS(NezhaUrl), result.Data.AgentSecret)
	return cmd, nil
}

func boolToTLS(url string) string {
	if strings.HasPrefix(url, "https") {
		return "true"
	}
	return "false"
}

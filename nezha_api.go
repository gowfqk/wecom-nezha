package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GetNezhaServerList 获取服务器列表
func GetNezhaServerList() ([]NezhaServer, error) {
	if NezhaUrl == "" || NezhaToken == "" {
		return nil, fmt.Errorf("Nezha配置未设置")
	}

	url := fmt.Sprintf("%s/api/v1/server/list", strings.TrimRight(NezhaUrl, "/"))
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+NezhaToken)

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

	if result.Code != 0 {
		return nil, fmt.Errorf("API错误: %s", result.Message)
	}

	data, err := json.Marshal(result.Result)
	if err != nil {
		return nil, err
	}

	var servers []NezhaServer
	if err := json.Unmarshal(data, &servers); err != nil {
		return nil, err
	}

	// 计算在线状态
	now := time.Now().Unix()
	for i := range servers {
		servers[i].Online = now-servers[i].LastActive < 300 // 5分钟内活跃视为在线
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

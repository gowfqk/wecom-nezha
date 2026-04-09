package main

import (
	"crypto/sha1"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// WecomCallbackHandler 处理企微回调验证和消息
func WecomCallbackHandler(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "text/plain")

	// 验证请求方法
	if req.Method == "GET" {
		// 验证回调 URL
		verifyCallback(res, req)
	} else if req.Method == "POST" {
		// 处理消息
		handleCallbackMessage(res, req)
	} else {
		http.Error(res, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// verifyCallback 验证回调 URL
func verifyCallback(res http.ResponseWriter, req *http.Request) {
	signature := req.URL.Query().Get("signature")
	timestamp := req.URL.Query().Get("timestamp")
	nonce := req.URL.Query().Get("nonce")
	echostr := req.URL.Query().Get("echostr")

	// 验证签名
	if !verifySignature(signature, timestamp, nonce, WecomToken) {
		http.Error(res, "签名验证失败", http.StatusForbidden)
		return
	}

	// 返回 echostr
	if WecomEncodingAESKey != "" {
		// 加密模式（简化处理）
		res.Write([]byte(echostr))
	} else {
		res.Write([]byte(echostr))
	}
}

// handleCallbackMessage 处理接收到的消息
func handleCallbackMessage(res http.ResponseWriter, req *http.Request) {
	signature := req.URL.Query().Get("signature")
	timestamp := req.URL.Query().Get("timestamp")
	nonce := req.URL.Query().Get("nonce")

	// 验证签名
	if !verifySignature(signature, timestamp, nonce, WecomToken) {
		http.Error(res, "签名验证失败", http.StatusForbidden)
		return
	}

	// 读取消息体
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return
	}

	// 解析 XML 消息
	var msg WecomCallbackMessage
	if err := xml.Unmarshal(body, &msg); err != nil {
		return
	}

	// 处理文本消息
	if msg.MsgType == "text" {
		response := processUserMessage(msg.Content)
		sendReplyMessage(msg.FromUserName, response)
	}

	// 返回成功
	res.Write([]byte("success"))
}

// verifySignature 验证签名
func verifySignature(signature, timestamp, nonce, token string) bool {
	strs := sort.StringSlice{token, timestamp, nonce}
	sort.Strings(strs)
	str := strings.Join(strs, "")
	hash := sha1.Sum([]byte(str))
	return fmt.Sprintf("%x", hash) == signature
}

// processUserMessage 处理用户消息，返回回复内容
func processUserMessage(content string) string {
	content = strings.TrimSpace(content)

	switch content {
	case "帮助", "help", "?":
		return `帮助信息：
- 状态：查看服务器在线状态
- 离线：查看离线服务器列表
- 列表：查看所有服务器
- 服务器名：查看指定服务器详情

发送任意关键词查询服务器状态`
	case "状态", "状态查询":
		return getServerStatusSummary()
	case "离线":
		return getOfflineServersList()
	case "列表", "list":
		return getServerList()
	default:
		// 尝试匹配服务器名
		detail := getServerDetail(content)
		if strings.Contains(detail, "未找到") {
			return "未知命令，发送 帮助 查看可用命令"
		}
		return detail
	}
}

// getServerStatusSummary 获取服务器状态摘要
func getServerStatusSummary() string {
	servers, err := GetNezhaServerList()
	if err != nil {
		return fmt.Sprintf("获取状态失败: %v", err)
	}

	online := 0
	offline := 0
	for _, s := range servers {
		if s.Online {
			online++
		} else {
			offline++
		}
	}

	return fmt.Sprintf("服务器状态：\n在线: %d\n离线: %d\n总计: %d", online, offline, len(servers))
}

// getOfflineServersList 获取离线服务器列表
func getOfflineServersList() string {
	servers, err := GetOfflineServers()
	if err != nil {
		return fmt.Sprintf("获取离线列表失败: %v", err)
	}

	if len(servers) == 0 {
		return "当前没有离线服务器"
	}

	result := "离线服务器列表：\n"
	for _, s := range servers {
		result += fmt.Sprintf("- %s (%s)\n", s.Name, s.ValidIP)
	}
	return result
}

// getServerList 获取服务器列表
func getServerList() string {
	servers, err := GetNezhaServerList()
	if err != nil {
		return fmt.Sprintf("获取列表失败: %v", err)
	}

	result := "服务器列表：\n"
	for _, s := range servers {
		status := "🟢在线"
		if !s.Online {
			status = "🔴离线"
		}
		result += fmt.Sprintf("- %s %s\n", s.Name, status)
	}
	return result
}

// getServerDetail 获取服务器详情
func getServerDetail(name string) string {
	server, err := GetNezhaServerByName(name)
	if err == nil {
		// 精确匹配成功，显示详情
		return formatServerDetail(server)
	}

	// 模糊匹配：同时匹配 Name、Note、Tag
	servers, err := GetNezhaServerList()
	if err != nil {
		return fmt.Sprintf("查询失败: %v", err)
	}

	var matched []NezhaServer
	lowerName := strings.ToLower(name)
	for _, s := range servers {
		if strings.Contains(strings.ToLower(s.Name), lowerName) ||
			strings.Contains(strings.ToLower(s.Note), lowerName) ||
			strings.Contains(strings.ToLower(s.Tag), lowerName) {
			matched = append(matched, s)
		}
	}

	if len(matched) == 0 {
		return fmt.Sprintf("未找到服务器: %s\n发送 帮助 查看命令", name)
	}

	if len(matched) == 1 {
		// 只有一个匹配，显示详情
		return formatServerDetail(&matched[0])
	}

	// 多个匹配，显示列表
	result := "找到多个匹配的服务器：\n"
	for _, m := range matched {
		status := "🟢在线"
		if !m.Online {
			status = "🔴离线"
		}
		result += fmt.Sprintf("- %s %s\n", m.Name, status)
	}
	result += "\n回复服务器名称查看详情"
	return result
}

// formatServerDetail 格式化服务器详情
func formatServerDetail(server *NezhaServer) string {
	status := "🟢在线"
	if !server.Online {
		status = "🔴离线"
	}

	return fmt.Sprintf(`服务器: %s
状态: %s
IP: %s
备注: %s
CPU: %.1f%%
内存: %d / %d GB
磁盘: %d / %d GB
负载: %.2f / %.2f / %.2f`,
		server.Name, status, server.ValidIP, server.Note,
		server.Status.CPU,
		server.Status.MemUsed/1024/1024/1024, server.Host.MemTotal/1024/1024/1024,
		server.Status.DiskUsed/1024/1024/1024, server.Host.DiskTotal/1024/1024/1024,
		server.Status.Load1, server.Status.Load5, server.Status.Load15)
}

// sendReplyMessage 发送回复消息
func sendReplyMessage(toUser, content string) {
	if content == "" {
		return
	}

	accessToken := GetAccessToken()
	if accessToken == "" {
		return
	}

	postData := JsonData{
		ToUser:   toUser,
		AgentId:  WecomAid,
		MsgType:  "text",
		Text:     Msg{Content: content},
	}

	url := fmt.Sprintf(SendMessageApi, accessToken)
	PostMsg(postData, url)
}

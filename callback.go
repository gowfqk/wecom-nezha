package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// processedMsgIDs 已处理的消息ID缓存，防止重复处理
var processedMsgIDs = struct {
	sync.RWMutex
	ids map[int64]time.Time
}{ids: make(map[int64]time.Time)}

func init() {
	// 单一后台 Ticker 定期清理过期消息 ID，避免每条消息都启动 goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			processedMsgIDs.Lock()
			for id, t := range processedMsgIDs.ids {
				if time.Since(t) > 5*time.Minute {
					delete(processedMsgIDs.ids, id)
				}
			}
			processedMsgIDs.Unlock()
		}
	}()
}

// isMsgProcessed 检查消息是否已处理
func isMsgProcessed(msgID int64) bool {
	processedMsgIDs.RLock()
	_, exists := processedMsgIDs.ids[msgID]
	processedMsgIDs.RUnlock()
	return exists
}

// markMsgProcessed 标记消息为已处理
func markMsgProcessed(msgID int64) {
	processedMsgIDs.Lock()
	processedMsgIDs.ids[msgID] = time.Now()
	processedMsgIDs.Unlock()
}

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
	echostr := strings.ReplaceAll(req.URL.Query().Get("echostr"), " ", "+")
	msgSignature := req.URL.Query().Get("msg_signature")

	logger.Printf("收到回调验证请求: signature=%s, timestamp=%s, nonce=%s, echostr=%s, msg_signature=%s", 
		signature, timestamp, nonce, echostr, msgSignature)

	// 判断是否加密模式
	if msgSignature != "" {
		// 加密模式
		decryptedEchostr, err := decryptMsg(echostr)
		if err != nil {
			logger.Printf("解密echostr失败: %v", err)
			http.Error(res, "解密失败", http.StatusForbidden)
			return
		}
		
		// 验证签名（加密模式用 decrypt_msg）
		if !verifyEncryptSignature(msgSignature, timestamp, nonce, echostr) {
			logger.Printf("加密模式签名验证失败")
			http.Error(res, "签名验证失败", http.StatusForbidden)
			return
		}
		
		logger.Println("加密模式回调验证成功")
		res.Write([]byte(decryptedEchostr))
	} else {
		// 明文模式
		if !verifySignature(signature, timestamp, nonce, WecomToken) {
			logger.Printf("明文模式签名验证失败")
			http.Error(res, "签名验证失败", http.StatusForbidden)
			return
		}
		logger.Println("明文模式回调验证成功")
		res.Write([]byte(echostr))
	}
}

// handleCallbackMessage 处理接收到的消息
func handleCallbackMessage(res http.ResponseWriter, req *http.Request) {
	signature := req.URL.Query().Get("signature")
	timestamp := req.URL.Query().Get("timestamp")
	nonce := req.URL.Query().Get("nonce")
	msgSignature := req.URL.Query().Get("msg_signature")

	logger.Printf("收到回调消息: signature=%s, timestamp=%s, nonce=%s, msg_signature=%s", 
		signature, timestamp, nonce, msgSignature)

	// 读取消息体
	body, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Printf("读取消息体失败: %v", err)
		return
	}
	logger.Printf("收到消息内容: %s", string(body))

	// 解析 XML 消息
	var msg WecomCallbackMessage
	if err := xml.Unmarshal(body, &msg); err != nil {
		logger.Printf("解析XML消息失败: %v", err)
		return
	}

	// 判断是否加密模式
	if msg.Encrypt != "" {
		// 验证加密模式签名
		if !verifyEncryptSignature(msgSignature, timestamp, nonce, msg.Encrypt) {
			logger.Println("加密模式消息签名验证失败")
			http.Error(res, "签名验证失败", http.StatusForbidden)
			return
		}
		
		// 解密消息
		decryptedContent, err := decryptMsg(msg.Encrypt)
		if err != nil {
			logger.Printf("解密消息失败: %v", err)
			return
		}
		
		// 解析解密后的消息
		var decryptedMsg WecomCallbackMessage
		if err := xml.Unmarshal([]byte(decryptedContent), &decryptedMsg); err != nil {
			logger.Printf("解析解密消息失败: %v", err)
			return
		}
		
		logger.Printf("收到用户消息(解密后): MsgType=%s, FromUser=%s, Content=%s", 
			decryptedMsg.MsgType, decryptedMsg.FromUserName, decryptedMsg.Content)
		
		// 检查消息是否已处理
		if decryptedMsg.MsgId > 0 && isMsgProcessed(decryptedMsg.MsgId) {
			logger.Printf("消息 %d 已处理，跳过", decryptedMsg.MsgId)
			res.Write([]byte("success"))
			return
		}
		
		// 处理文本消息
		if decryptedMsg.MsgType == "text" {
			// 标记消息为已处理
			if decryptedMsg.MsgId > 0 {
				markMsgProcessed(decryptedMsg.MsgId)
			}
			
			response := processUserMessage(decryptedMsg.Content, decryptedMsg.FromUserName)
			logger.Printf("发送回复: %s", response)
			sendReplyMessage(decryptedMsg.FromUserName, response)
		}
	} else {
		// 明文模式
		if !verifySignature(signature, timestamp, nonce, WecomToken) {
			logger.Println("明文模式消息签名验证失败")
			http.Error(res, "签名验证失败", http.StatusForbidden)
			return
		}

		logger.Printf("收到用户消息: MsgType=%s, FromUser=%s, Content=%s",
			msg.MsgType, msg.FromUserName, msg.Content)

		// 检查消息是否已处理
		if msg.MsgId > 0 && isMsgProcessed(msg.MsgId) {
			logger.Printf("消息 %d 已处理，跳过", msg.MsgId)
			res.Write([]byte("success"))
			return
		}
		
		// 处理文本消息
		if msg.MsgType == "text" {
			// 标记消息为已处理
			if msg.MsgId > 0 {
				markMsgProcessed(msg.MsgId)
			}
			
			response := processUserMessage(msg.Content, msg.FromUserName)
			logger.Printf("发送回复: %s", response)
			sendReplyMessage(msg.FromUserName, response)
		}
	}

	// 返回成功
	res.Write([]byte("success"))
}

// verifySignature 验证明文签名
func verifySignature(signature, timestamp, nonce, token string) bool {
	strs := sort.StringSlice{token, timestamp, nonce}
	sort.Strings(strs)
	str := strings.Join(strs, "")
	hash := sha1.Sum([]byte(str))
	return fmt.Sprintf("%x", hash) == signature
}

// verifyEncryptSignature 验证加密模式签名
func verifyEncryptSignature(msgSignature, timestamp, nonce, encryptMsg string) bool {
	strs := sort.StringSlice{WecomToken, timestamp, nonce, encryptMsg}
	sort.Strings(strs)
	str := strings.Join(strs, "")
	hash := sha1.Sum([]byte(str))
	return fmt.Sprintf("%x", hash) == msgSignature
}

// decryptMsg 解密消息（参考微信官方Python示例）
func decryptMsg(encryptMsg string) (string, error) {
	if WecomEncodingAESKey == "" {
		return "", fmt.Errorf("EncodingAESKey未配置")
	}

	// Base64 解码密文
	encryptedBytes, err := base64.StdEncoding.DecodeString(encryptMsg)
	if err != nil {
		return "", fmt.Errorf("Base64解码失败: %v", err)
	}

	// 解码 AES Key (43字符 -> 32字节)，加一个'='补齐
	aesKey, err := base64.StdEncoding.DecodeString(WecomEncodingAESKey + "=")
	if err != nil {
		return "", fmt.Errorf("AES Key解码失败: %v", err)
	}
	if len(aesKey) != 32 {
		return "", fmt.Errorf("AES Key长度错误: 期望32字节, 实际%d字节", len(aesKey))
	}

	// AES 解密（IV = AES Key 前16字节，参考微信官方示例）
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("创建AES cipher失败: %v", err)
	}

	if len(encryptedBytes)%block.BlockSize() != 0 {
		return "", fmt.Errorf("密文长度不是块大小的整数倍: %d", len(encryptedBytes))
	}

	iv := aesKey[:16]
	decrypted := make([]byte, len(encryptedBytes))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(decrypted, encryptedBytes)

	// 微信消息解密不用PKCS7，直接用msg_len字段定位消息

	// 格式: random(16) + msg_len(4) + msg + corp_id
	if len(decrypted) < 20 {
		return "", fmt.Errorf("解密后数据过短: %d字节", len(decrypted))
	}
	msgLen := int(decrypted[16])<<24 | int(decrypted[17])<<16 | int(decrypted[18])<<8 | int(decrypted[19])
	if msgLen < 0 || 20+msgLen > len(decrypted) {
		return "", fmt.Errorf("消息长度无效: %d（Token或EncodingAESKey可能不正确）", msgLen)
	}
	msg := decrypted[20 : 20+msgLen]

	return string(msg), nil
}

// pendingAction 待确认操作
type pendingAction struct {
	Type string                 // "nat_add", "nat_delete"
	Data map[string]interface{} // 操作数据
}

var pendingActions = make(map[string]pendingAction) // userID -> action
var pendingMutex sync.RWMutex

// formatMatchedList 格式化多个匹配服务器的列表提示
func formatMatchedList(matched []NezhaServer, suffix string) string {
	result := "找到多个匹配的服务器：\n"
	for _, m := range matched {
		status := "🟢在线"
		if !m.Online {
			status = "🔴离线"
		}
		result += fmt.Sprintf("- %s %s %s\n", m.Name, summarizeTag(m.Tag), status)
	}
	if suffix != "" {
		result += "\n" + suffix
	}
	return result
}

// resolveServer 使用 FindServer 查找服务器并返回格式化的错误消息
// 返回: server（成功时）, 错误提示消息（失败时）
func resolveServer(name string, matchTag bool) (*NezhaServer, string) {
	result, err := FindServer(name, matchTag)
	if err != nil {
		return nil, fmt.Sprintf("查询服务器失败: %v", err)
	}
	if result.Server != nil {
		return result.Server, ""
	}
	if len(result.Matched) > 0 {
		return nil, formatMatchedList(result.Matched, "请用更精确的名称")
	}
	return nil, fmt.Sprintf("未找到服务器: %s", name)
}

// processUserMessage 处理用户消息，返回回复内容
func processUserMessage(content, userID string) string {
	content = strings.TrimSpace(content)

	// 检查是否有待确认操作
	if content == "确认" || content == "取消" {
		return handleConfirmAction(content, userID)
	}

	switch content {
	case "帮助", "help", "?":
		return `帮助信息：
- 状态：查看服务器在线状态
- 离线：查看离线服务器列表
- 列表：查看所有服务器
- 安装：获取Agent安装命令
- 详情 服务器名：查看服务器完整信息
- 监控 服务器名 [指标] [周期]：查看监控历史
- 服务：查看服务监控状态
- 重启 服务器名：重启服务器（需确认）
- 服务器名：快速查看服务器状态
- NAT：查看穿透配置列表
- NAT 添加：分步添加穿透配置
- NAT 启用/禁用 ID：启用或禁用穿透
- NAT 删除 ID：删除穿透配置（需确认）
- NAT 修改 ID 内网地址:端口 [服务器名]
- DDNS：查看DDNS配置列表
- DDNS 添加：分步添加DDNS配置
- DDNS 删除 ID：删除DDNS配置（需确认）
- DDNS 启用/禁用 ID：启用或禁用IPv4/IPv6
- DDNS 提供商：查看支持的DDNS提供商
- 通知：查看通知渠道列表
- 通知 添加 名称 URL：快速添加通知渠道
- 通知 删除 ID：删除通知渠道（需确认）
- 标签 服务器名 标签内容：更新服务器私有备注/标签
- 确认/取消：通用确认机制

监控指标: cpu/memory/disk/net_in_speed/net_out_speed/load1
监控周期: 1d(默认)/7d/30d`
	case "状态", "状态查询":
		return getServerStatusSummary()
	case "离线":
		return getOfflineServersList()
	case "列表", "list":
		return getServerList()
	case "服务", "service":
		return getServiceStatus()
	case "安装", "agent":
		return `安装命令用法：
- 安装 linux：Linux 一键安装
- 安装 windows：Windows 安装命令
- 安装 docker：Docker 安装命令`
	default:
		lower := strings.ToLower(content)

		// NAT 命令
		if lower == "nat" || lower == "nat 列表" {
			return getNatList()
		}
		if strings.HasPrefix(lower, "nat 添加") {
			return startNatAdd(userID, content)
		}
		if strings.HasPrefix(lower, "nat 删除") {
			return startNatDelete(userID, content)
		}
		if strings.HasPrefix(lower, "nat 启用") || strings.HasPrefix(lower, "nat 禁用") {
			return toggleNatCmd(content)
		}
		if strings.HasPrefix(lower, "nat 修改") {
			return updateNatCmd(content)
		}

		// DDNS 命令
		if lower == "ddns" || lower == "ddns 列表" {
			return getDDNSList()
		}
		if lower == "ddns 提供商" || lower == "ddns providers" {
			return getDDNSProviders()
		}
		if strings.HasPrefix(lower, "ddns 添加") {
			return startDDNSAdd(userID, content)
		}
		if strings.HasPrefix(lower, "ddns 删除") {
			return startDDNSDelete(userID, content)
		}
		if strings.HasPrefix(lower, "ddns 启用") || strings.HasPrefix(lower, "ddns 禁用") {
			return toggleDDNSCmd(content)
		}

		// 通知渠道命令
		if lower == "通知" || lower == "通知 列表" || lower == "notification" {
			return getNotificationList()
		}
		if strings.HasPrefix(lower, "通知 添加") || strings.HasPrefix(lower, "通知 新增") {
			return startNotificationAdd(userID, content)
		}
		if strings.HasPrefix(lower, "通知 删除") {
			return startNotificationDelete(userID, content)
		}

		// 标签/备注命令
		if strings.HasPrefix(lower, "标签 ") || strings.HasPrefix(lower, "标签\t") ||
			strings.HasPrefix(lower, "备注 ") || strings.HasPrefix(lower, "备注\t") {
			return updateServerNoteCmd(content)
		}

		// 检查是否有待确认的 NAT 添加步骤（含 step 字段表示分步进行中）
		pendingMutex.RLock()
		action, hasPending := pendingActions[userID]
		pendingMutex.RUnlock()
		if hasPending && action.Type == "nat_add" {
			if _, hasStep := action.Data["step"]; hasStep {
				return handleNatAddStep(userID, content)
			}
		}
		if hasPending && action.Type == "ddns_add" {
			if _, hasStep := action.Data["step"]; hasStep {
				return handleDDNSAddStep(userID, content)
			}
		}
		if hasPending && action.Type == "notification_add" {
			if _, hasStep := action.Data["step"]; hasStep {
				return handleNotificationAddStep(userID, content)
			}
		}

		// 安装命令带平台参数
		if strings.HasPrefix(lower, "安装 ") || strings.HasPrefix(lower, "安装") {
			platform := strings.TrimSpace(strings.TrimPrefix(lower, "安装"))
			if platform == "" {
				return `安装命令用法：
- 安装 linux：Linux 一键安装
- 安装 windows：Windows 安装命令
- 安装 docker：Docker 安装命令`
			}
			return getAgentInstallCmd(platform)
		}

		// 监控历史命令
		if strings.HasPrefix(lower, "监控 ") || strings.HasPrefix(lower, "monitor ") {
			return getServerMetricsCmd(content)
		}

		// 详情命令
		if strings.HasPrefix(lower, "详情 ") {
			name := strings.TrimSpace(strings.TrimPrefix(lower, "详情 "))
			return getServerDetailFull(name)
		}

		// 重启命令
		if strings.HasPrefix(lower, "重启 ") {
			name := strings.TrimSpace(strings.TrimPrefix(lower, "重启 "))
			return restartServer(name, userID)
		}

		// 尝试匹配服务器名（快速查看）
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

// getAgentInstallCmd 获取 Agent 安装命令
func getAgentInstallCmd(platform string) string {
	secret, err := GetAgentSecret()
	if err != nil {
		return fmt.Sprintf("获取安装命令失败: %v", err)
	}

	tls := "false"
	if strings.HasPrefix(NezhaUrl, "https") {
		tls = "true"
	}

	// NZ_SERVER 需要 host:port 格式，去掉协议前缀
	serverAddr := strings.TrimPrefix(strings.TrimPrefix(NezhaUrl, "https://"), "http://")
	
	// 如果没有指定端口，根据协议添加默认端口
	if !strings.Contains(serverAddr, ":") {
		if strings.HasPrefix(NezhaUrl, "https") {
			serverAddr += ":443"
		} else if strings.HasPrefix(NezhaUrl, "http") {
			serverAddr += ":80"
		}
	}

	switch platform {
	case "linux":
		cmd := fmt.Sprintf("curl -L https://raw.githubusercontent.com/nezhahq/scripts/main/agent/install.sh -o agent.sh && chmod +x agent.sh && env NZ_SERVER=%s NZ_TLS=%s NZ_CLIENT_SECRET=%s ./agent.sh",
			serverAddr, tls, secret)
		return fmt.Sprintf("Linux 安装命令：\n\n%s\n\n⚠️ 请在目标服务器上以 root 权限执行", cmd)
	case "windows":
		return fmt.Sprintf("Windows 安装命令（PowerShell）：\n\n"+
			"$env:NZ_SERVER=\"%s\";"+
			"$env:NZ_TLS=\"%s\";"+
			"$env:NZ_CLIENT_SECRET=\"%s\"; "+
			"[Net.ServicePointManager]::SecurityProtocol = "+
			"[Net.SecurityProtocolType]::Ssl3 -bor "+
			"[Net.SecurityProtocolType]::Tls -bor "+
			"[Net.SecurityProtocolType]::Tls11 -bor "+
			"[Net.SecurityProtocolType]::Tls12;"+
			"set-ExecutionPolicy RemoteSigned;"+
			"Invoke-WebRequest https://f.o0oo.cc/s/nezha.ps1 -OutFile C:\\nezha.ps1;"+
			"powershell.exe C:\\nezha.ps1",
			serverAddr, tls, secret)
	case "docker":
		return fmt.Sprintf("Docker 安装命令：\n\n"+
			"docker run -d \\\n"+
			"  --name nezha-agent \\\n"+
			"  --restart=always \\\n"+
			"  --net=host \\\n"+
			"  -v ./nezha-data:/nezha/agent/data \\\n"+
			"  -e NZ_SERVER=%s \\\n"+
			"  -e NZ_CLIENT_SECRET=%s \\\n"+
			"  -e NZ_TLS=%s \\\n"+
			"  nezhahq/agent:latest",
			serverAddr, secret, tls)
	default:
		return fmt.Sprintf("不支持的平台: %s\n支持: linux / windows / docker", platform)
	}
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
		result += fmt.Sprintf("- %s %s %s\n", s.Name, summarizeTag(s.Tag), status)
	}
	return result
}

// getServerDetail 获取服务器详情
func getServerDetail(name string) string {
	result, err := FindServer(name, true)
	if err != nil {
		return fmt.Sprintf("查询失败: %v", err)
	}
	if result.Server != nil {
		return formatServerDetail(result.Server)
	}
	if len(result.Matched) > 0 {
		return formatMatchedList(result.Matched, "回复服务器名称查看详情")
	}
	return fmt.Sprintf("未找到服务器: %s\n发送 帮助 查看命令", name)
}

// formatServerDetail 格式化服务器详情（快速查看）
func formatServerDetail(server *NezhaServer) string {
	status := "🟢在线"
	if !server.Online {
		status = "🔴离线"
	}

	memPct := float64(0)
	if server.Host.MemTotal > 0 {
		memPct = float64(server.State.MemUsed) / float64(server.Host.MemTotal) * 100
	}
	diskPct := float64(0)
	if server.Host.DiskTotal > 0 {
		diskPct = float64(server.State.DiskUsed) / float64(server.Host.DiskTotal) * 100
	}

	return fmt.Sprintf(`服务器: %s
状态: %s
IP: %s
备注: %s
CPU: %.1f%%
内存: %d / %d GB (%.1f%%)
磁盘: %d / %d GB (%.1f%%)
标签: %s`,
		server.Name, status, server.ValidIP, summarizeNote(server.Note),
		server.State.CPU,
		server.State.MemUsed/1024/1024/1024, server.Host.MemTotal/1024/1024/1024, memPct,
		server.State.DiskUsed/1024/1024/1024, server.Host.DiskTotal/1024/1024/1024, diskPct,
		summarizeTag(server.Tag))
}

// getLoad 获取负载值，Windows 下无法获取时返回 N/A
func getLoad(server *NezhaServer) (string, string, string) {
	l1, l5, l15 := server.State.Load1, server.State.Load5, server.State.Load15
	isWindows := strings.Contains(strings.ToLower(server.Host.Platform), "windows")
	if isWindows && l1 == 0 && l5 == 0 && l15 == 0 {
		return "N/A", "N/A", "N/A"
	}
	return fmt.Sprintf("%.2f", l1), fmt.Sprintf("%.2f", l5), fmt.Sprintf("%.2f", l15)
}

// formatServerDetailFull 格式化服务器完整详情
func formatServerDetailFull(server *NezhaServer) string {
	status := "🟢在线"
	if !server.Online {
		status = "🔴离线"
	}

	// CPU 型号
	cpuModel := "未知"
	if len(server.Host.CPU) > 0 {
		cpuModel = strings.Join(server.Host.CPU, ", ")
	}

	// 运行时间
	uptime := formatDuration(server.State.Uptime)

	// 网速
	netIn := formatSpeed(server.State.NetInSpeed)
	netOut := formatSpeed(server.State.NetOutSpeed)

	// 总流量
	netInTotal := formatBytes(server.State.NetInTransfer)
	netOutTotal := formatBytes(server.State.NetOutTransfer)

	// Agent 版本
	agentVer := server.Host.Version
	if agentVer == "" {
		agentVer = "未知"
	}

	l1, l5, l15 := getLoad(server)

	return fmt.Sprintf(`服务器: %s
状态: %s | 运行: %s
系统: %s %s (%s)
CPU: %s
内存: %d / %d GB (%.1f%%)
磁盘: %d / %d GB (%.1f%%)
负载: %s / %s / %s
网络: ↓%s ↑%s (累计 ↓%s ↑%s)
连接: TCP %d / UDP %d
进程: %d
Agent: %s
IP: %s
备注: %s
标签: %s`,
		server.Name, status, uptime,
		server.Host.Platform, server.Host.PlatformVersion, server.Host.Arch,
		cpuModel,
		server.State.MemUsed/1024/1024/1024, server.Host.MemTotal/1024/1024/1024,
		float64(server.State.MemUsed)/float64(server.Host.MemTotal)*100,
		server.State.DiskUsed/1024/1024/1024, server.Host.DiskTotal/1024/1024/1024,
		float64(server.State.DiskUsed)/float64(server.Host.DiskTotal)*100,
		l1, l5, l15,
		netIn, netOut, netInTotal, netOutTotal,
		server.State.TCPConnCount, server.State.UDPConnCount,
		server.State.ProcessCount,
		agentVer,
		server.ValidIP, summarizeNote(server.Note), summarizeTag(server.Tag))
}

// getServerDetailFull 查找并显示服务器完整详情
func getServerDetailFull(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "用法: 详情 服务器名"
	}
	result, err := FindServer(name, false)
	if err != nil {
		return fmt.Sprintf("查询失败: %v", err)
	}
	if result.Server != nil {
		return formatServerDetailFull(result.Server)
	}
	if len(result.Matched) > 0 {
		return formatMatchedList(result.Matched, "请用完整名称查看详情")
	}
	return fmt.Sprintf("未找到服务器: %s", name)
}

// restartServer 重启服务器（通过创建触发任务）
func restartServer(name, userID string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "用法: 重启 服务器名"
	}
	
	server, errMsg := resolveServer(name, true)
	if errMsg != "" {
		return errMsg
	}
	
	if !server.Online {
		return fmt.Sprintf("服务器 %s 当前离线，无法重启", server.Name)
	}
	
	// 需要确认 - 保存待确认操作
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "restart",
		Data: map[string]interface{}{
			"server_id": server.ID,
			"server":    server.Name,
			"platform":  server.Host.Platform,
		},
	}
	pendingMutex.Unlock()
	
	return fmt.Sprintf(`确定要重启服务器吗？
服务器: %s
标签: %s
IP: %s
状态: 🟢在线

⚠️ 重启将导致服务器短暂中断
回复 确认 重启，回复 取消 放弃`,
		server.Name, summarizeTag(server.Tag), server.ValidIP)
}

// formatDuration 格式化运行时间
func formatDuration(seconds uint64) string {
	if seconds == 0 {
		return "未知"
	}
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60
	if days > 0 {
		return fmt.Sprintf("%d天%d小时%d分", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d小时%d分", hours, minutes)
	}
	return fmt.Sprintf("%d分钟", minutes)
}

// formatSpeed 格式化网速
func formatSpeed(bytesPerSec float64) string {
	if bytesPerSec >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB/s", bytesPerSec/1024/1024/1024)
	}
	if bytesPerSec >= 1024*1024 {
		return fmt.Sprintf("%.1f MB/s", bytesPerSec/1024/1024)
	}
	if bytesPerSec >= 1024 {
		return fmt.Sprintf("%.1f KB/s", bytesPerSec/1024)
	}
	return fmt.Sprintf("%.0f B/s", bytesPerSec)
}

// formatBytes 格式化字节数
func formatBytes(bytes uint64) string {
	if bytes >= 1024*1024*1024*1024 {
		return fmt.Sprintf("%.1f TB", float64(bytes)/1024/1024/1024/1024)
	}
	if bytes >= 1024*1024*1024 {
		return fmt.Sprintf("%.1f GB", float64(bytes)/1024/1024/1024)
	}
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	}
	return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
}

// sendReplyMessage 发送回复消息
func sendReplyMessage(toUser, content string) {
	if content == "" {
		logger.Println("回复内容为空，跳过发送")
		return
	}

	logger.Printf("准备发送消息给用户: %s", toUser)

	accessToken := GetAccessToken()
	if accessToken == "" {
		logger.Println("获取access_token失败")
		return
	}

	postData := JsonData{
		ToUser:   toUser,
		AgentId:  WecomAid,
		MsgType:  "text",
		Text:     Msg{Content: content},
	}

	url := fmt.Sprintf(SendMessageApi, accessToken)
	logger.Printf("发送消息URL: %s", url)
	
	result := PostMsg(postData, url)
	logger.Printf("发送消息结果: %s", result)
}

// getNatList 获取 NAT 列表
func getNatList() string {
	nats, err := GetNatList()
	if err != nil {
		return fmt.Sprintf("获取NAT列表失败: %v", err)
	}
	if len(nats) == 0 {
		return "当前没有NAT穿透配置\n发送 NAT 添加 开始配置"
	}

	result := "NAT 穿透配置列表：\n"
	for _, n := range nats {
		id := uint(n["id"].(float64))
		name := n["name"].(string)
		domain := n["domain"].(string)
		host := n["host"].(string)
		enabled := n["enabled"].(bool)
		status := "🟢启用"
		if !enabled {
			status = "🔴禁用"
		}
		// 获取服务器ID
		serverID := uint(0)
		if sid, ok := n["server_id"]; ok && sid != nil {
			serverID = uint(sid.(float64))
		}
		serverInfo := ""
		if serverID > 0 {
			serverInfo = fmt.Sprintf(" → 服务器:%d", serverID)
		}
		result += fmt.Sprintf("- [%d] %s %s%s\n  %s → %s\n", id, name, status, serverInfo, domain, host)
	}
	result += "\n操作：NAT 启用/禁用 ID | NAT 删除 ID | NAT 修改 ID 内网地址:端口 [服务器名]"
	return result
}

// startNatAdd 开始分步添加 NAT
func startNatAdd(userID, content string) string {
	// 格式: NAT 添加 名称 域名 内网地址:端口 服务器名
	parts := strings.Fields(content)
	if len(parts) == 5 {
		// 一步到位
		name := parts[1]
		domain := parts[2]
		host := parts[3]
		serverName := parts[4]
		return confirmNatAdd(userID, name, domain, host, serverName)
	}

	// 分步引导
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "nat_add",
		Data: map[string]interface{}{"step": float64(0)},
	}
	pendingMutex.Unlock()

	return "开始添加 NAT 穿透配置：\n\n第 1 步：请输入配置名称\n（如：SSH穿透、Web服务）"
}

// handleNatAddStep 处理 NAT 添加的分步输入
func handleNatAddStep(userID, content string) string {
	pendingMutex.Lock()
	action := pendingActions[userID]
	pendingMutex.Unlock()

	data := action.Data
	step := int(data["step"].(float64))

	switch step {
	case 0: // 输入名称
		data["name"] = content
		data["step"] = float64(1)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "nat_add", Data: data}
		pendingMutex.Unlock()
		return "第 2 步：请输入外网域名\n（如：ssh.example.com）"

	case 1: // 输入域名
		data["domain"] = content
		data["step"] = float64(2)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "nat_add", Data: data}
		pendingMutex.Unlock()
		return "第 3 步：请输入内网地址和端口\n（如：192.168.1.100:22）"

	case 2: // 输入内网地址
		data["host"] = content
		data["step"] = float64(3)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "nat_add", Data: data}
		pendingMutex.Unlock()
		return "第 4 步：请输入关联的服务器名称\n（支持模糊匹配）"

	case 3: // 输入服务器名 → 确认
		pendingMutex.Lock()
		delete(pendingActions, userID)
		pendingMutex.Unlock()
		return confirmNatAdd(userID,
			data["name"].(string),
			data["domain"].(string),
			data["host"].(string),
			content)
	}
	return "配置异常，请重新发送 NAT 添加"
}

// confirmNatAdd 确认添加 NAT
func confirmNatAdd(userID, name, domain, host, serverName string) string {
	server, errMsg := resolveServer(serverName, false)
	if errMsg != "" {
		return errMsg + "\n请重新发送 NAT 添加"
	}

	// 保存待确认操作
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "nat_add",
		Data: map[string]interface{}{
			"name":      name,
			"domain":    domain,
			"host":      host,
			"server_id": server.ID,
			"server":    server.Name,
		},
	}
	pendingMutex.Unlock()

	return fmt.Sprintf(`请确认 NAT 配置：
名称: %s
域名: %s
内网: %s
服务器: %s

回复 确认 创建，回复 取消 放弃`, name, domain, host, server.Name)
}

// handleConfirmAction 处理确认/取消操作
func handleConfirmAction(content, userID string) string {
	pendingMutex.Lock()
	action, ok := pendingActions[userID]
	if ok {
		delete(pendingActions, userID)
	}
	pendingMutex.Unlock()

	if !ok {
		return "没有待确认的操作"
	}

	if content == "取消" {
		return "已取消操作"
	}

	// 确认
	switch action.Type {
	case "nat_add":
		data := action.Data
		var serverID uint
		switch v := data["server_id"].(type) {
		case uint:
			serverID = v
		case float64:
			serverID = uint(v)
		}
		err := AddNat(
			data["name"].(string),
			data["domain"].(string),
			data["host"].(string),
			serverID,
		)
		if err != nil {
			return fmt.Sprintf("添加NAT失败: %v", err)
		}
		return fmt.Sprintf("✅ NAT 配置已创建\n名称: %s\n域名: %s → %s",
			data["name"], data["domain"], data["host"])

	case "nat_delete":
		var id uint
		switch v := action.Data["id"].(type) {
		case uint:
			id = v
		case float64:
			id = uint(v)
		}
		name := action.Data["name"].(string)
		err := DeleteNat(id)
		if err != nil {
			return fmt.Sprintf("删除NAT失败: %v", err)
		}
		return fmt.Sprintf("✅ NAT 配置已删除: %s", name)
	
	case "restart":
		var serverID uint
		switch v := action.Data["server_id"].(type) {
		case uint:
			serverID = v
		case float64:
			serverID = uint(v)
		}
		serverName := action.Data["server"].(string)
		platform := action.Data["platform"].(string)
		err := RebootNezhaServer(serverID, platform)
		if err != nil {
			return fmt.Sprintf("重启 %s 失败: %v", serverName, err)
		}
		return fmt.Sprintf("✅ 已向 %s 发送重启指令", serverName)

	case "ddns_delete":
		var id uint
		switch v := action.Data["id"].(type) {
		case uint:
			id = v
		case float64:
			id = uint(v)
		}
		name := action.Data["name"].(string)
		err := DeleteDDNS(id)
		if err != nil {
			return fmt.Sprintf("删除DDNS失败: %v", err)
		}
		return fmt.Sprintf("✅ DDNS 配置已删除: %s", name)

	case "ddns_add":
		data := action.Data
		domains := []string{}
		if d, ok := data["domains"].(string); ok {
			domains = strings.Split(d, ",")
		}
		enableIPv4 := true
		enableIPv6 := false
		if v, ok := data["enable_ipv4"].(bool); ok {
			enableIPv4 = v
		}
		if v, ok := data["enable_ipv6"].(bool); ok {
			enableIPv6 = v
		}
		err := AddDDNS(
			data["name"].(string),
			data["provider"].(string),
			data["access_id"].(string),
			data["access_secret"].(string),
			domains,
			enableIPv4,
			enableIPv6,
		)
		if err != nil {
			return fmt.Sprintf("添加DDNS失败: %v", err)
		}
		return fmt.Sprintf("✅ DDNS 配置已创建\n名称: %s\n提供商: %s\n域名: %s",
			data["name"], data["provider"], data["domains"])

	case "notification_delete":
		var id uint
		switch v := action.Data["id"].(type) {
		case uint:
			id = v
		case float64:
			id = uint(v)
		}
		name := action.Data["name"].(string)
		err := DeleteNotification(id)
		if err != nil {
			return fmt.Sprintf("删除通知渠道失败: %v", err)
		}
		return fmt.Sprintf("✅ 通知渠道已删除: %s", name)

	case "notification_add":
		data := action.Data
		err := AddNotification(
			data["name"].(string),
			data["url"].(string),
			uint(1), // POST
			uint(1), // JSON
			"Content-Type: application/json",
			data["request_body"].(string),
		)
		if err != nil {
			return fmt.Sprintf("添加通知渠道失败: %v", err)
		}
		return fmt.Sprintf("✅ 通知渠道已创建\n名称: %s\nURL: %s", data["name"], data["url"])
	}

	return "操作异常"
}

// startNatDelete 开始删除 NAT（需确认）
func startNatDelete(userID, content string) string {
	parts := strings.Fields(content)
	if len(parts) < 3 {
		return "用法: NAT 删除 ID\n发送 NAT 查看配置列表和ID"
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return "ID 无效，请输入数字\n发送 NAT 查看配置列表"
	}

	// 查找配置名称
	nats, err := GetNatList()
	if err != nil {
		return fmt.Sprintf("查询NAT列表失败: %v", err)
	}
	var natName string
	found := false
	for _, n := range nats {
		if uint(n["id"].(float64)) == uint(id) {
			natName = n["name"].(string)
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("未找到 ID=%d 的NAT配置\n发送 NAT 查看列表", id)
	}

	// 保存待确认
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "nat_delete",
		Data: map[string]interface{}{
			"id":   float64(id),
			"name": natName,
		},
	}
	pendingMutex.Unlock()

	return fmt.Sprintf("确定要删除 NAT 配置 [%d] %s 吗？\n回复 确认 删除，回复 取消 放弃", id, natName)
}

// toggleNatCmd 启用/禁用 NAT
func toggleNatCmd(content string) string {
	parts := strings.Fields(content)
	if len(parts) < 3 {
		return "用法: NAT 启用 ID / NAT 禁用 ID\n发送 NAT 查看配置列表"
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return "ID 无效，请输入数字"
	}

	enabled := strings.ToLower(parts[1]) == "启用"
	err = ToggleNat(uint(id), enabled)
	if err != nil {
		return fmt.Sprintf("操作失败: %v", err)
	}

	action := "启用"
	if !enabled {
		action = "禁用"
	}
	return fmt.Sprintf("✅ NAT [%d] 已%s", id, action)
}

// updateNatCmd 修改 NAT 配置（内网地址和/或服务器）
// 格式: 
//   NAT 修改 ID 内网地址:端口 [服务器名]
//   NAT 修改 ID - 服务器名  (只改服务器，不改地址)
func updateNatCmd(content string) string {
	parts := strings.Fields(content)
	if len(parts) < 4 {
		return "用法: NAT 修改 ID 内网地址:端口 [服务器名]\n" +
			"     NAT 修改 ID - 服务器名  (只改服务器)\n" +
			"如: NAT 修改 1 192.168.1.100:8080\n" +
			"    NAT 修改 1 - 安宁三小\n" +
			"发送 NAT 查看配置列表和ID"
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return "ID 无效，请输入数字\n发送 NAT 查看配置列表"
	}

	var host string
	var serverID uint

	// 判断是只改服务器还是改地址+服务器
	if parts[3] == "-" {
		// 只改服务器: NAT 修改 ID - 服务器名
		if len(parts) < 5 {
			return "请提供服务器名\n用法: NAT 修改 ID - 服务器名"
		}
		serverName := strings.TrimSpace(parts[4])
		server, errMsg := resolveServer(serverName, true)
		if errMsg != "" {
			return errMsg
		}
		serverID = server.ID
	} else {
		// 改地址，可选改服务器
		host = strings.TrimSpace(parts[3])
		if !strings.Contains(host, ":") {
			return "格式错误，请使用 内网地址:端口 格式\n如: 192.168.1.100:8080"
		}

		// 如果还有第5个参数，是服务器名
		if len(parts) >= 5 {
			serverName := strings.TrimSpace(parts[4])
			server, errMsg := resolveServer(serverName, true)
			if errMsg != "" {
				return errMsg
			}
			serverID = server.ID
		}
	}

	err = UpdateNat(uint(id), host, serverID)
	if err != nil {
		return fmt.Sprintf("修改失败: %v", err)
	}

	msg := fmt.Sprintf("✅ NAT [%d] 已更新", id)
	if host != "" {
		msg += fmt.Sprintf("\n新地址: %s", host)
	}
	if serverID > 0 {
		msg += fmt.Sprintf("\n新服务器: ID=%d", serverID)
	}
	return msg
}

// updateServerNoteCmd 更新服务器私有备注/标签
// 格式: 标签 服务器名 标签内容 或 备注 服务器名 标签内容
func updateServerNoteCmd(content string) string {
	lower := strings.ToLower(content)
	var trimmed string
	
	// 处理"标签"或"备注"前缀
	if strings.HasPrefix(lower, "标签 ") || strings.HasPrefix(lower, "标签\t") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(content, "标签"))
	} else if strings.HasPrefix(lower, "备注 ") || strings.HasPrefix(lower, "备注\t") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(content, "备注"))
	} else {
		return "用法: 标签 服务器名 标签内容\n如: 标签 安宁三小 主域控"
	}
	
	// 用第一个空格分隔服务器名和标签内容
	fields := strings.SplitN(trimmed, " ", 2)
	if len(fields) < 2 || strings.TrimSpace(fields[1]) == "" {
		return "用法: 标签 服务器名 标签内容\n如: 标签 安宁三小 主域控"
	}

	serverName := strings.TrimSpace(fields[0])
	note := strings.TrimSpace(fields[1])

	// 查找服务器
	server, errMsg := resolveServer(serverName, true)
	if errMsg != "" {
		return errMsg
	}

	err := UpdateServerTag(server.ID, note)
	if err != nil {
		return fmt.Sprintf("更新标签失败: %v", err)
	}
	return fmt.Sprintf("✅ 已更新 %s 的标签\n标签: %s", server.Name, note)
}


// getServerMetricsCmd 获取服务器监控历史
// 格式: 监控 服务器名 [指标] [周期]
// 指标: cpu(默认)/memory/disk/net_in_speed/net_out_speed/load1/load5/load15
// 周期: 1d(默认)/7d/30d
func getServerMetricsCmd(content string) string {
	lower := strings.ToLower(content)
	var trimmed string
	if strings.HasPrefix(lower, "监控 ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(content, "监控"))
	} else if strings.HasPrefix(lower, "monitor ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(lower, "monitor "))
	}

	if trimmed == "" {
		return `监控命令用法：
- 监控 服务器名：查看 CPU 24h 数据
- 监控 服务器名 memory：查看内存数据
- 监控 服务器名 cpu 7d：查看 CPU 7天数据

支持指标: cpu/memory/disk/net_in_speed/net_out_speed/load1/load5/load15
支持周期: 1d(默认)/7d/30d`
	}

	parts := strings.Fields(trimmed)
	serverName := parts[0]
	metric := "cpu"
	period := "1d"

	if len(parts) >= 2 {
		metric = strings.ToLower(parts[1])
	}
	if len(parts) >= 3 {
		period = strings.ToLower(parts[2])
	}

	// 验证指标名
	validMetrics := map[string]string{
		"cpu":              "CPU 使用率",
		"memory":           "内存使用率",
		"disk":             "磁盘使用率",
		"swap":             "Swap 使用率",
		"net_in_speed":     "入站网速",
		"net_out_speed":    "出站网速",
		"net_in_transfer":  "入站总流量",
		"net_out_transfer": "出站总流量",
		"load1":            "1分钟负载",
		"load5":            "5分钟负载",
		"load15":           "15分钟负载",
	}
	metricLabel, ok := validMetrics[metric]
	if !ok {
		return fmt.Sprintf("不支持的指标: %s\n支持: cpu/memory/disk/swap/net_in_speed/net_out_speed/load1/load5/load15", metric)
	}

	// 验证周期
	if period != "1d" && period != "7d" && period != "30d" {
		return "不支持的周期，可选: 1d / 7d / 30d"
	}

	// 查找服务器
	server, errMsg := resolveServer(serverName, true)
	if errMsg != "" {
		return errMsg
	}

	// 获取监控数据
	dataPoints, err := GetServerMetrics(server.ID, metric, period)
	if err != nil {
		return fmt.Sprintf("获取监控数据失败: %v", err)
	}

	if len(dataPoints) == 0 {
		return fmt.Sprintf("服务器 %s 暂无 %s 的监控数据（%s）", server.Name, metricLabel, period)
	}

	// 计算统计值
	var sum, maxVal, minVal float64
	minVal = dataPoints[0].Avg
	for _, dp := range dataPoints {
		sum += dp.Avg
		if dp.Avg > maxVal {
			maxVal = dp.Avg
		}
		if dp.Avg < minVal {
			minVal = dp.Avg
		}
	}
	avgVal := sum / float64(len(dataPoints))
	current := dataPoints[len(dataPoints)-1].Avg

	// 获取总量用于计算占比
	var total uint64
	switch metric {
	case "memory":
		total = server.Host.MemTotal
	case "disk":
		total = server.Host.DiskTotal
	case "swap":
		total = server.Host.SwapTotal
	}

	// 格式化输出：根据指标类型选择合适的显示方式
	formatVal := func(v float64) string {
		switch {
		case metric == "cpu":
			// CPU 使用率，API 返回 0-100 的百分比
			return fmt.Sprintf("%.2f%%", v)
		case metric == "memory" || metric == "disk" || metric == "swap":
			// 内存/磁盘/Swap：API 可能返回百分比(0-100)或字节数(>100)
			if v > 100 && total > 0 {
				// 返回的是字节数，同时显示已用量和百分比
				pct := v / float64(total) * 100
				return fmt.Sprintf("%s / %s (%.1f%%)", formatBytes(uint64(v)), formatBytes(total), pct)
			} else if v > 100 {
				return formatBytes(uint64(v))
			}
			return fmt.Sprintf("%.2f%%", v)
		case strings.Contains(metric, "speed"):
			return formatSpeed(v)
		case strings.Contains(metric, "transfer"):
			return formatBytes(uint64(v))
		case strings.Contains(metric, "load"):
			return fmt.Sprintf("%.2f", v)
		default:
			return fmt.Sprintf("%.2f", v)
		}
	}

	return fmt.Sprintf(`监控数据: %s - %s（%s）
当前: %s
平均: %s
最高: %s
最低: %s
数据点: %d 个`,
		server.Name, metricLabel, period,
		formatVal(current),
		formatVal(avgVal),
		formatVal(maxVal),
		formatVal(minVal),
		len(dataPoints))
}

// getServiceStatus 获取服务监控状态
func getServiceStatus() string {
	services, err := GetServiceList()
	if err != nil {
		return fmt.Sprintf("获取服务状态失败: %v", err)
	}

	if len(services) == 0 {
		return "当前没有配置服务监控\n请在哪吒面板中添加服务监控"
	}

	// 服务类型映射
	typeNames := map[uint]string{
		0: "TCP",
		1: "HTTP",
		2: "HTTPS",
		3: "DNS",
		4: "端口",
		5: "Ping",
	}

	online := 0
	offline := 0
	result := "服务监控状态：\n"

	for _, s := range services {
		status := "🟢"
		if !s.Online {
			status = "🔴"
			offline++
		} else {
			online++
		}

		typeName := typeNames[s.Type]
		if typeName == "" {
			typeName = "未知"
		}

		delayStr := "N/A"
		if s.AvgDelay > 0 {
			if s.AvgDelay > 1000 {
				delayStr = fmt.Sprintf("%.1fs", s.AvgDelay/1000)
			} else {
				delayStr = fmt.Sprintf("%.0fms", s.AvgDelay)
			}
		}

		result += fmt.Sprintf("%s [%s] %s → %s (%s)\n", status, typeName, s.Name, s.Target, delayStr)
	}

	result += fmt.Sprintf("\n总计: %d | 正常: %d | 异常: %d", len(services), online, offline)
	return result
}



// ========== DDNS 管理命令 ==========

// getDDNSList 获取 DDNS 配置列表
func getDDNSList() string {
	ddnsList, err := GetDDNSList()
	if err != nil {
		return fmt.Sprintf("获取DDNS列表失败: %v", err)
	}
	if len(ddnsList) == 0 {
		return "当前没有DDNS配置\n发送 DDNS 添加 开始配置"
	}

	result := "DDNS 配置列表：\n"
	for _, d := range ddnsList {
		id := uint(0)
		if v, ok := d["id"].(float64); ok {
			id = uint(v)
		}
		name, _ := d["name"].(string)
		provider, _ := d["provider"].(string)

		// 域名列表
		domainStr := ""
		if domains, ok := d["domains"].([]interface{}); ok {
			for i, dom := range domains {
				if i > 0 {
					domainStr += ", "
				}
				domainStr += fmt.Sprintf("%v", dom)
			}
		}

		ipv4 := "❌"
		if v, ok := d["enable_ipv4"].(bool); ok && v {
			ipv4 = "✅"
		}
		ipv6 := "❌"
		if v, ok := d["enable_ipv6"].(bool); ok && v {
			ipv6 = "✅"
		}

		result += fmt.Sprintf("- [%d] %s (%s)\n  域名: %s\n  IPv4: %s | IPv6: %s\n",
			id, name, provider, domainStr, ipv4, ipv6)
	}
	result += "\n操作：DDNS 添加 | DDNS 删除 ID | DDNS 启用/禁用 ID"
	return result
}

// getDDNSProviders 获取 DDNS 提供商列表
func getDDNSProviders() string {
	providers, err := GetDDNSProviders()
	if err != nil {
		return fmt.Sprintf("获取DDNS提供商列表失败: %v", err)
	}
	if len(providers) == 0 {
		return "未获取到DDNS提供商列表"
	}

	result := "支持的DDNS提供商：\n"
	for _, p := range providers {
		result += fmt.Sprintf("- %s\n", p)
	}
	result += "\n使用 DDNS 添加 开始配置"
	return result
}

// startDDNSAdd 开始添加 DDNS 配置（分步引导）
func startDDNSAdd(userID, content string) string {
	// 分步引导
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "ddns_add",
		Data: map[string]interface{}{"step": float64(0)},
	}
	pendingMutex.Unlock()

	return "开始添加 DDNS 配置：\n\n第 1 步：请输入配置名称\n（如：我的DDNS、主域名解析）"
}

// handleDDNSAddStep 处理 DDNS 添加的分步输入
func handleDDNSAddStep(userID, content string) string {
	pendingMutex.Lock()
	action := pendingActions[userID]
	pendingMutex.Unlock()

	data := action.Data
	step := int(data["step"].(float64))

	switch step {
	case 0: // 输入名称
		data["name"] = content
		data["step"] = float64(1)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "ddns_add", Data: data}
		pendingMutex.Unlock()

		// 获取提供商列表提示
		providers, err := GetDDNSProviders()
		hint := "第 2 步：请输入DDNS提供商\n"
		if err == nil && len(providers) > 0 {
			hint += "支持: " + strings.Join(providers, ", ")
		} else {
			hint += "（如：aliyun, cloudflare, dnspod, namesilo 等）"
		}
		return hint

	case 1: // 输入提供商
		data["provider"] = strings.TrimSpace(content)
		data["step"] = float64(2)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "ddns_add", Data: data}
		pendingMutex.Unlock()
		return "第 3 步：请输入 Access ID（API Key）"

	case 2: // 输入 Access ID
		data["access_id"] = strings.TrimSpace(content)
		data["step"] = float64(3)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "ddns_add", Data: data}
		pendingMutex.Unlock()
		return "第 4 步：请输入 Access Secret（API Secret）"

	case 3: // 输入 Access Secret
		data["access_secret"] = strings.TrimSpace(content)
		data["step"] = float64(4)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "ddns_add", Data: data}
		pendingMutex.Unlock()
		return "第 5 步：请输入域名（多个域名用逗号分隔）\n（如：example.com 或 a.example.com,b.example.com）"

	case 4: // 输入域名 → 确认
		data["domains"] = strings.TrimSpace(content)
		data["enable_ipv4"] = true
		data["enable_ipv6"] = false
		// 删除 step 字段，进入确认阶段
		delete(data, "step")
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "ddns_add", Data: data}
		pendingMutex.Unlock()

		return fmt.Sprintf(`请确认 DDNS 配置：
名称: %s
提供商: %s
域名: %s
IPv4: ✅ | IPv6: ❌

回复 确认 创建，回复 取消 放弃`,
			data["name"], data["provider"], data["domains"])
	}
	return "配置异常，请重新发送 DDNS 添加"
}

// startDDNSDelete 开始删除 DDNS（需确认）
func startDDNSDelete(userID, content string) string {
	parts := strings.Fields(content)
	if len(parts) < 3 {
		return "用法: DDNS 删除 ID\n发送 DDNS 查看配置列表和ID"
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return "ID 无效，请输入数字\n发送 DDNS 查看配置列表"
	}

	// 查找配置名称
	ddnsList, err := GetDDNSList()
	if err != nil {
		return fmt.Sprintf("查询DDNS列表失败: %v", err)
	}
	var ddnsName string
	found := false
	for _, d := range ddnsList {
		if uint(d["id"].(float64)) == uint(id) {
			ddnsName, _ = d["name"].(string)
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("未找到 ID=%d 的DDNS配置\n发送 DDNS 查看列表", id)
	}

	// 保存待确认
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "ddns_delete",
		Data: map[string]interface{}{
			"id":   float64(id),
			"name": ddnsName,
		},
	}
	pendingMutex.Unlock()

	return fmt.Sprintf("确定要删除 DDNS 配置 [%d] %s 吗？\n回复 确认 删除，回复 取消 放弃", id, ddnsName)
}

// toggleDDNSCmd 启用/禁用 DDNS 的 IPv4/IPv6
// 格式: DDNS 启用 ID 或 DDNS 禁用 ID
func toggleDDNSCmd(content string) string {
	parts := strings.Fields(content)
	if len(parts) < 3 {
		return "用法: DDNS 启用 ID / DDNS 禁用 ID\n启用=开启IPv4，禁用=关闭IPv4\n发送 DDNS 查看配置列表"
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return "ID 无效，请输入数字"
	}

	enabled := strings.Contains(parts[1], "启用")
	updateData := map[string]interface{}{
		"enable_ipv4": enabled,
	}

	err = UpdateDDNS(uint(id), updateData)
	if err != nil {
		return fmt.Sprintf("操作失败: %v", err)
	}

	action := "启用IPv4"
	if !enabled {
		action = "禁用IPv4"
	}
	return fmt.Sprintf("✅ DDNS [%d] 已%s", id, action)
}

// ========== 通知渠道管理命令 ==========

// getNotificationList 获取通知渠道列表
func getNotificationList() string {
	notifications, err := GetNotificationList()
	if err != nil {
		return fmt.Sprintf("获取通知渠道列表失败: %v", err)
	}
	if len(notifications) == 0 {
		return "当前没有通知渠道配置\n发送 通知 添加 开始配置"
	}

	result := "通知渠道列表：\n"
	for _, n := range notifications {
		id := uint(0)
		if v, ok := n["id"].(float64); ok {
			id = uint(v)
		}
		name, _ := n["name"].(string)
		notifyURL, _ := n["url"].(string)

		// 截断 URL 显示
		displayURL := notifyURL
		if len(displayURL) > 50 {
			displayURL = displayURL[:47] + "..."
		}

		result += fmt.Sprintf("- [%d] %s\n  URL: %s\n", id, name, displayURL)
	}
	result += "\n操作：通知 添加 名称 URL | 通知 删除 ID"
	return result
}

// startNotificationAdd 开始添加通知渠道
// 支持快速模式: 通知 添加 名称 URL
// 或分步引导
func startNotificationAdd(userID, content string) string {
	parts := strings.Fields(content)
	// 快速模式: 通知 添加 名称 URL
	if len(parts) >= 4 {
		name := parts[2]
		notifyURL := parts[3]
		if !strings.HasPrefix(notifyURL, "http") {
			return "URL 格式错误，请以 http:// 或 https:// 开头"
		}
		// 默认请求体模板
		defaultBody := `{"msgtype":"text","text":{"content":"${msg}"}}`

		// 保存待确认
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{
			Type: "notification_add",
			Data: map[string]interface{}{
				"name":         name,
				"url":          notifyURL,
				"request_body": defaultBody,
			},
		}
		pendingMutex.Unlock()

		return fmt.Sprintf(`请确认通知渠道配置：
名称: %s
URL: %s
请求体: %s

回复 确认 创建，回复 取消 放弃
提示: 创建后可在哪吒面板中修改请求体模板`, name, notifyURL, defaultBody)
	}

	// 分步引导
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "notification_add",
		Data: map[string]interface{}{"step": float64(0)},
	}
	pendingMutex.Unlock()

	return "开始添加通知渠道：\n\n第 1 步：请输入通知名称\n（如：钉钉通知、企微机器人）"
}

// handleNotificationAddStep 处理通知渠道添加的分步输入
func handleNotificationAddStep(userID, content string) string {
	pendingMutex.Lock()
	action := pendingActions[userID]
	pendingMutex.Unlock()

	data := action.Data
	step := int(data["step"].(float64))

	switch step {
	case 0: // 输入名称
		data["name"] = content
		data["step"] = float64(1)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "notification_add", Data: data}
		pendingMutex.Unlock()
		return "第 2 步：请输入 Webhook URL\n（如：https://oapi.dingtalk.com/robot/send?access_token=xxx）"

	case 1: // 输入 URL
		notifyURL := strings.TrimSpace(content)
		if !strings.HasPrefix(notifyURL, "http") {
			return "URL 格式错误，请以 http:// 或 https:// 开头\n请重新输入 URL"
		}
		data["url"] = notifyURL
		data["step"] = float64(2)
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "notification_add", Data: data}
		pendingMutex.Unlock()
		return `第 3 步：请输入请求体模板（JSON格式）
使用 ${msg} 作为消息占位符

示例（钉钉）:
{"msgtype":"text","text":{"content":"${msg}"}}

直接回复模板，或回复 默认 使用上述模板`

	case 2: // 输入请求体模板 → 确认
		body := strings.TrimSpace(content)
		if body == "默认" || body == "default" {
			body = `{"msgtype":"text","text":{"content":"${msg}"}}`
		}
		data["request_body"] = body
		// 删除 step 字段，进入确认阶段
		delete(data, "step")
		pendingMutex.Lock()
		pendingActions[userID] = pendingAction{Type: "notification_add", Data: data}
		pendingMutex.Unlock()

		return fmt.Sprintf(`请确认通知渠道配置：
名称: %s
URL: %s
请求体: %s

回复 确认 创建，回复 取消 放弃`,
			data["name"], data["url"], body)
	}
	return "配置异常，请重新发送 通知 添加"
}

// startNotificationDelete 开始删除通知渠道（需确认）
func startNotificationDelete(userID, content string) string {
	parts := strings.Fields(content)
	if len(parts) < 3 {
		return "用法: 通知 删除 ID\n发送 通知 查看列表和ID"
	}

	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return "ID 无效，请输入数字\n发送 通知 查看列表"
	}

	// 查找名称
	notifications, err := GetNotificationList()
	if err != nil {
		return fmt.Sprintf("查询通知列表失败: %v", err)
	}
	var notifyName string
	found := false
	for _, n := range notifications {
		if uint(n["id"].(float64)) == uint(id) {
			notifyName, _ = n["name"].(string)
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf("未找到 ID=%d 的通知渠道\n发送 通知 查看列表", id)
	}

	// 保存待确认
	pendingMutex.Lock()
	pendingActions[userID] = pendingAction{
		Type: "notification_delete",
		Data: map[string]interface{}{
			"id":   float64(id),
			"name": notifyName,
		},
	}
	pendingMutex.Unlock()

	return fmt.Sprintf("确定要删除通知渠道 [%d] %s 吗？\n回复 确认 删除，回复 取消 放弃", id, notifyName)
}

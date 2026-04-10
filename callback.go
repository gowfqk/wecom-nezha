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
			// 调试：打印密文前20字符和长度
			logger.Printf("调试: Encrypt长度=%d, 前20字符=%q", len(msg.Encrypt), msg.Encrypt[:20])
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
		
		// 处理文本消息
		if decryptedMsg.MsgType == "text" {
			response := processUserMessage(decryptedMsg.Content)
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
		
		// 处理文本消息
		if msg.MsgType == "text" {
			response := processUserMessage(msg.Content)
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

	// 调试：打印解密后末尾16字节
	if len(decrypted) >= 16 {
		logger.Printf("调试: 解密后末尾16字节=%v", decrypted[len(decrypted)-16:])
	}

	// 去除 PKCS7 填充（带校验）
	decrypted, err = pkcs7Unpad(decrypted)
	if err != nil {
		return "", fmt.Errorf("解密失败，Token或EncodingAESKey可能不正确: %v", err)
	}

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

// pkcs7Unpad 去除 PKCS7 填充（带校验）
func pkcs7Unpad(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, fmt.Errorf("数据为空")
	}
	unpadding := int(data[length-1])
	if unpadding <= 0 || unpadding > 16 || unpadding > length {
		return nil, fmt.Errorf("无效的PKCS7填充: %d", unpadding)
	}
	// 校验填充字节是否一致
	for i := length - unpadding; i < length; i++ {
		if data[i] != byte(unpadding) {
			return nil, fmt.Errorf("PKCS7填充校验失败")
		}
	}
	return data[:length-unpadding], nil
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

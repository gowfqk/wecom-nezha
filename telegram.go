package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Telegram 配置
var TelegramBotToken = GetEnvDefault("TELEGRAM_BOT_TOKEN", "")
var TelegramWebhookSecret = GetEnvDefault("TELEGRAM_WEBHOOK_SECRET", "")
var TelegramAllowedUsers = GetEnvDefault("TELEGRAM_ALLOWED_USERS", "") // 逗号分隔的用户ID列表，空表示允许所有
var TelegramAPIBase = GetEnvDefault("TELEGRAM_API_BASE", "https://api.telegram.org")

// Telegram API 基础 URL
func getTelegramAPIBase() string {
	return strings.TrimRight(TelegramAPIBase, "/") + "/bot" + TelegramBotToken
}

// TelegramUpdate Telegram 更新结构
type TelegramUpdate struct {
	UpdateID      int                   `json:"update_id"`
	Message       *TelegramMessage      `json:"message,omitempty"`
	CallbackQuery *TelegramCallbackQuery `json:"callback_query,omitempty"`
}

// TelegramMessage Telegram 消息结构
type TelegramMessage struct {
	MessageID int          `json:"message_id"`
	From      TelegramUser `json:"from"`
	Chat      TelegramChat `json:"chat"`
	Text      string       `json:"text"`
	Timestamp int          `json:"date"`
}

// TelegramUser Telegram 用户结构
type TelegramUser struct {
	ID        int    `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

// TelegramChat Telegram 聊天结构
type TelegramChat struct {
	ID    int    `json:"id"`
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
}

// TelegramCallbackQuery Telegram 回调查询
type TelegramCallbackQuery struct {
	ID      string           `json:"id"`
	From    TelegramUser     `json:"from"`
	Message *TelegramMessage `json:"message,omitempty"`
	Data    string           `json:"data"`
}

// TelegramInlineKeyboard 内联键盘
type TelegramInlineKeyboard struct {
	InlineKeyboardMarkup [][]TelegramInlineKeyboardButton `json:"inline_keyboard"`
}

// TelegramInlineKeyboardButton 内联键盘按钮
type TelegramInlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// 已处理的消息ID缓存（防止重复处理）
var processedTelegramMsgIDs = struct {
	sync.RWMutex
	ids map[int]time.Time
}{ids: make(map[int]time.Time)}

// pendingEdit 待编辑操作（用户点了编辑按钮后等待输入新值）
type pendingEdit struct {
	Field      string // "name", "note", "public_note"
	ServerName string
	ServerID   uint
}

var pendingEdits = struct {
	sync.RWMutex
	edits map[int]pendingEdit // userID -> pendingEdit
}{edits: make(map[int]pendingEdit)}

func init() {
	// 启动后台清理 goroutine
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			processedTelegramMsgIDs.Lock()
			for id, t := range processedTelegramMsgIDs.ids {
				if time.Since(t) > 5*time.Minute {
					delete(processedTelegramMsgIDs.ids, id)
				}
			}
			processedTelegramMsgIDs.Unlock()
		}
	}()
}

// isTelegramMsgProcessed 检查消息是否已处理
func isTelegramMsgProcessed(msgID int) bool {
	processedTelegramMsgIDs.RLock()
	_, exists := processedTelegramMsgIDs.ids[msgID]
	processedTelegramMsgIDs.RUnlock()
	return exists
}

// markTelegramMsgProcessed 标记消息为已处理
func markTelegramMsgProcessed(msgID int) {
	processedTelegramMsgIDs.Lock()
	processedTelegramMsgIDs.ids[msgID] = time.Now()
	processedTelegramMsgIDs.Unlock()
}

// isUserAllowed 检查用户是否被允许
func isUserAllowed(userID int) bool {
	if TelegramAllowedUsers == "" {
		return true // 空表示允许所有
	}

	userIDStr := strconv.Itoa(userID)
	for _, id := range strings.Split(TelegramAllowedUsers, ",") {
		if strings.TrimSpace(id) == userIDStr {
			return true
		}
	}
	return false
}

// TelegramWebhookHandler 处理 Telegram Webhook
func TelegramWebhookHandler(res http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(res, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 可选：验证 webhook secret
	if TelegramWebhookSecret != "" {
		secret := req.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		if secret != TelegramWebhookSecret {
			logger.Printf("Telegram webhook secret 验证失败")
			http.Error(res, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		logger.Printf("读取 Telegram 请求体失败: %v", err)
		http.Error(res, "Bad Request", http.StatusBadRequest)
		return
	}

	var update TelegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		logger.Printf("解析 Telegram 消息失败: %v", err)
		http.Error(res, "Bad Request", http.StatusBadRequest)
		return
	}

	// 处理消息
	if update.Message != nil {
		go handleTelegramMessage(update.Message)
	}

	// 处理回调查询
	if update.CallbackQuery != nil {
		go handleTelegramCallback(update.CallbackQuery)
	}

	res.WriteHeader(http.StatusOK)
	res.Write([]byte(`{"ok":true}`))
}

// handleTelegramMessage 处理 Telegram 消息
func handleTelegramMessage(msg *TelegramMessage) {
	// 检查消息是否已处理
	if isTelegramMsgProcessed(msg.MessageID) {
		return
	}
	markTelegramMsgProcessed(msg.MessageID)

	// 检查用户权限
	if !isUserAllowed(msg.From.ID) {
		logger.Printf("Telegram 用户 %d (%s) 未被允许", msg.From.ID, msg.From.Username)
		return
	}

	// 忽略非文本消息
	if msg.Text == "" {
		return
	}

	content := strings.TrimSpace(msg.Text)
	userID := fmt.Sprintf("telegram_%d", msg.From.ID)

	logger.Printf("收到 Telegram 消息: 用户=%d (%s), 内容=%s", msg.From.ID, msg.From.Username, content)

	// 检查是否有待编辑操作（用户点了编辑按钮后发送新值）
	pendingEdits.RLock()
	edit, hasEdit := pendingEdits.edits[msg.From.ID]
	pendingEdits.RUnlock()
	if hasEdit {
		// 清除待编辑状态
		pendingEdits.Lock()
		delete(pendingEdits.edits, msg.From.ID)
		pendingEdits.Unlock()

		// 执行更新
		fieldNames := map[string]string{
			"name":        "名称",
			"note":        "标签",
			"public_note": "备注",
		}
		err := UpdateServerField(edit.ServerID, edit.Field, content)
		if err != nil {
			sendTelegramMessage(msg.Chat.ID, fmt.Sprintf("❌ 更新失败: %v", err), nil)
			return
		}
		sendTelegramMessage(msg.Chat.ID, fmt.Sprintf("✅ 已更新 %s 的%s\n%s: %s",
			edit.ServerName, fieldNames[edit.Field], fieldNames[edit.Field], content), nil)
		return
	}

	// 处理命令
	var response string
	var keyboard *TelegramInlineKeyboard

	// 特殊命令处理
	switch {
	case content == "/start", content == "/help", content == "帮助", content == "help", content == "?":
		response = getTelegramHelpMessage()
	case content == "/status", content == "状态", content == "状态查询":
		response = getServerStatusSummary()
	case content == "/list", content == "列表", content == "list":
		response = getServerList()
		// 添加 inline keyboard 供快速选择
		keyboard = buildServerKeyboard()
	case content == "/offline", content == "离线":
		response = getOfflineServersList()
	case content == "/service", content == "服务", content == "service":
		response = getServiceStatus()
	case content == "/nat", content == "nat", content == "NAT":
		response = getNatList()
	case content == "/ddns", content == "ddns", content == "DDNS":
		response = getDDNSList()
	case content == "/notification", content == "通知", content == "notification":
		response = getNotificationList()
	case content == "/install", content == "安装", content == "agent":
		response = `安装命令用法：
- 安装 linux：Linux 一键安装
- 安装 windows：Windows 安装命令
- 安装 docker：Docker 安装命令`
	default:
		// 单词输入：先尝试作为服务器名查询（带编辑键盘）
		if !strings.Contains(content, " ") {
			detail, exactName := getServerDetailWithExactName(content)
			if exactName != "" {
				response = detail
				keyboard = buildEditKeyboard(exactName)
				break
			}
		}
		// 不是服务器名或包含空格，走通用消息处理
		response = processUserMessage(content, userID)
	}

	// 发送回复
	if response != "" {
		sendTelegramMessage(msg.Chat.ID, response, keyboard)
	}
}

// handleTelegramCallback 处理 Telegram 回调查询
func handleTelegramCallback(callback *TelegramCallbackQuery) {
	// 检查用户权限
	if !isUserAllowed(callback.From.ID) {
		logger.Printf("Telegram 回调用户 %d 未被允许", callback.From.ID)
		return
	}

	data := callback.Data
	userID := fmt.Sprintf("telegram_%d", callback.From.ID)

	logger.Printf("收到 Telegram 回调: 用户=%d, 数据=%s", callback.From.ID, data)

	var response string
	var keyboard *TelegramInlineKeyboard

	// 处理回调数据
	switch {
	case data == "cmd:status":
		response = getServerStatusSummary()
	case data == "cmd:list":
		response = getServerList()
		keyboard = buildServerKeyboard()
	case data == "cmd:offline":
		response = getOfflineServersList()
	case data == "cmd:service":
		response = getServiceStatus()
	case data == "cmd:help":
		response = getTelegramHelpMessage()
	case strings.HasPrefix(data, "server:"):
		// 查询服务器详情
		serverName := strings.TrimPrefix(data, "server:")
		var exactName string
		response, exactName = getServerDetailWithExactName(serverName)
		if exactName != "" {
			keyboard = buildEditKeyboard(exactName)
		}
	case strings.HasPrefix(data, "edit:"):
		// 编辑字段: edit:field:serverName
		parts := strings.SplitN(strings.TrimPrefix(data, "edit:"), ":", 2)
		if len(parts) == 2 {
			field := parts[0]
			serverName := parts[1]
			fieldNames := map[string]string{
				"name":        "名称",
				"note":        "标签",
				"public_note": "备注",
			}
			fieldName, ok := fieldNames[field]
			if !ok {
				response = "未知字段"
				break
			}
			// 查找服务器ID
			server, err := GetNezhaServerByName(serverName)
			if err != nil {
				response = fmt.Sprintf("未找到服务器: %s", serverName)
				break
			}
			// 保存待编辑状态
			pendingEdits.Lock()
			pendingEdits.edits[callback.From.ID] = pendingEdit{
				Field:      field,
				ServerName: server.Name,
				ServerID:   server.ID,
			}
			pendingEdits.Unlock()
			response = fmt.Sprintf("📝 请输入 %s 的新%s：", server.Name, fieldName)
		} else {
			response = "无效的编辑操作"
		}
	case strings.HasPrefix(data, "confirm:"):
		// 处理确认操作
		response = handleConfirmAction("确认", userID)
	case data == "cancel":
		response = handleConfirmAction("取消", userID)
	default:
		// 尝试作为服务器名查询
		var exactName string
		response, exactName = getServerDetailWithExactName(data)
		if strings.Contains(response, "未找到") {
			response = "未知操作: " + data
		} else if exactName != "" {
			keyboard = buildEditKeyboard(exactName)
		}
	}

	// 回应回调查询
	answerTelegramCallback(callback.ID, "")

	// 发送或编辑消息
	if response != "" {
		if callback.Message != nil {
			editTelegramMessage(callback.Message.Chat.ID, callback.Message.MessageID, response, keyboard)
		} else {
			sendTelegramMessage(callback.From.ID, response, keyboard)
		}
	}
}

// getTelegramHelpMessage 获取 Telegram 帮助消息
func getTelegramHelpMessage() string {
	return `🤖 Nezha 监控 Bot

━━━━━━ 📊 服务器监控 ━━━━━━
/status - 服务器状态概览
/list - 所有服务器列表
/offline - 离线服务器
/service - 服务监控状态
<服务器名> - 快速查看详情
详情 <服务器名> - 完整信息
监控 <服务器名> [指标] [周期] - 监控历史

━━━━━━ 🔧 服务器管理 ━━━━━━
重启 <服务器名> - 重启服务器
安装 linux - Linux 安装命令
安装 windows - Windows 安装命令
安装 docker - Docker 安装命令
标签 <服务器名> <内容> - 更新标签
修改 <服务器名> <字段> <值> - 修改服务器信息

━━━━━━ 🌐 NAT 穿透 ━━━━━━
NAT - 查看穿透列表
NAT 添加 - 添加穿透配置
NAT 启用 <ID> - 启用穿透
NAT 禁用 <ID> - 禁用穿透
NAT 删除 <ID> - 删除穿透
NAT 修改 <ID> <地址:端口> [服务器]

━━━━━━ 🔄 DDNS 管理 ━━━━━━
DDNS - 查看 DDNS 列表
DDNS 添加 - 添加 DDNS 配置
DDNS 删除 <ID> - 删除 DDNS
DDNS 启用 <ID> - 启用 IPv4
DDNS 禁用 <ID> - 禁用 IPv4
DDNS 提供商 - 查看提供商

━━━━━━ 📢 通知渠道 ━━━━━━
通知 - 查看通知渠道
通知 添加 <名称> <URL> - 快速添加
通知 添加 - 分步添加
通知 删除 <ID> - 删除渠道

━━━━━━ 📋 监控参数 ━━━━━━
指标: cpu / memory / disk
      net_in_speed / net_out_speed / load1
周期: 1d (默认) / 7d / 30d

━━━━━━ ⚙️ 其他命令 ━━━━━━
确认 / 取消 - 确认操作
帮助 / help - 显示此帮助`
}

// buildServerKeyboard 构建服务器选择键盘
func buildServerKeyboard() *TelegramInlineKeyboard {
	servers, err := GetNezhaServerList()
	if err != nil {
		return nil
	}

	var buttons [][]TelegramInlineKeyboardButton
	for _, s := range servers {
		status := "🟢"
		if !s.Online {
			status = "🔴"
		}
		btn := TelegramInlineKeyboardButton{
			Text:         fmt.Sprintf("%s %s", status, s.Name),
			CallbackData: fmt.Sprintf("server:%s", s.Name),
		}
		buttons = append(buttons, []TelegramInlineKeyboardButton{btn})
	}

	if len(buttons) == 0 {
		return nil
	}

	// 添加常用操作按钮
	buttons = append(buttons, []TelegramInlineKeyboardButton{
		{Text: "📊 状态概览", CallbackData: "cmd:status"},
		{Text: "🔴 离线列表", CallbackData: "cmd:offline"},
	})
	buttons = append(buttons, []TelegramInlineKeyboardButton{
		{Text: "🔄 刷新列表", CallbackData: "cmd:list"},
		{Text: "❓ 帮助", CallbackData: "cmd:help"},
	})

	return &TelegramInlineKeyboard{InlineKeyboardMarkup: buttons}
}

// buildEditKeyboard 构建编辑操作键盘
func buildEditKeyboard(serverName string) *TelegramInlineKeyboard {
	buttons := [][]TelegramInlineKeyboardButton{
		{
			{Text: "✏️ 修改名称", CallbackData: fmt.Sprintf("edit:name:%s", serverName)},
			{Text: "🏷️ 修改标签", CallbackData: fmt.Sprintf("edit:note:%s", serverName)},
		},
		{
			{Text: "📝 修改备注", CallbackData: fmt.Sprintf("edit:public_note:%s", serverName)},
		},
		{
			{Text: "🔙 返回列表", CallbackData: "cmd:list"},
		},
	}
	return &TelegramInlineKeyboard{InlineKeyboardMarkup: buttons}
}

// sendTelegramMessage 发送 Telegram 消息
func sendTelegramMessage(chatID int, text string, keyboard *TelegramInlineKeyboard) {
	url := fmt.Sprintf("%s/sendMessage", getTelegramAPIBase())

	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}

	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Printf("序列化 Telegram 消息失败: %v", err)
		return
	}

	resp, err := httpClient.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Printf("发送 Telegram 消息失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Printf("Telegram API 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}
}

// editTelegramMessage 编辑 Telegram 消息
func editTelegramMessage(chatID int, messageID int, text string, keyboard *TelegramInlineKeyboard) {
	url := fmt.Sprintf("%s/editMessageText", getTelegramAPIBase())

	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}

	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Printf("序列化 Telegram 编辑消息失败: %v", err)
		return
	}

	resp, err := httpClient.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Printf("编辑 Telegram 消息失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Printf("Telegram 编辑消息 API 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}
}

// answerTelegramCallback 回应 Telegram 回调查询
func answerTelegramCallback(callbackID string, text string) {
	url := fmt.Sprintf("%s/answerCallbackQuery", getTelegramAPIBase())

	payload := map[string]interface{}{
		"callback_query_id": callbackID,
	}
	if text != "" {
		payload["text"] = text
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		logger.Printf("序列化 Telegram 回调回应失败: %v", err)
		return
	}

	resp, err := httpClient.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		logger.Printf("回应 Telegram 回调失败: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Printf("Telegram 回调回应 API 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}
}

// SetTelegramWebhook 设置 Telegram Webhook
func SetTelegramWebhook(webhookURL string) error {
	if TelegramBotToken == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN 未设置")
	}

	url := fmt.Sprintf("%s/setWebhook", getTelegramAPIBase())

	payload := map[string]interface{}{
		"url": webhookURL,
	}

	if TelegramWebhookSecret != "" {
		payload["secret_token"] = TelegramWebhookSecret
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化 webhook 配置失败: %v", err)
	}

	resp, err := httpClient.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("设置 webhook 失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram API 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		OK          bool   `json:"ok"`
		Result      bool   `json:"result"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.OK {
		return fmt.Errorf("Telegram API 返回失败: %s", result.Description)
	}

	logger.Printf("Telegram Webhook 设置成功: %s", webhookURL)
	return nil
}

// TelegramBotCommand Telegram Bot 命令结构
type TelegramBotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// SetTelegramBotCommands 设置 Telegram Bot 命令菜单
func SetTelegramBotCommands() error {
	if TelegramBotToken == "" {
		return fmt.Errorf("TELEGRAM_BOT_TOKEN 未设置")
	}

	url := fmt.Sprintf("%s/setMyCommands", getTelegramAPIBase())

	commands := []TelegramBotCommand{
		{Command: "status", Description: "📊 服务器状态概览"},
		{Command: "list", Description: "📋 所有服务器列表"},
		{Command: "offline", Description: "🔴 离线服务器"},
		{Command: "service", Description: "🔍 服务监控状态"},
		{Command: "nat", Description: "🌐 NAT 穿透列表"},
		{Command: "ddns", Description: "🔄 DDNS 配置列表"},
		{Command: "notification", Description: "📢 通知渠道列表"},
		{Command: "install", Description: "📦 Agent 安装命令"},
		{Command: "help", Description: "❓ 显示帮助信息"},
	}

	payload := map[string]interface{}{
		"commands": commands,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化命令配置失败: %v", err)
	}

	resp, err := httpClient.Post(url, "application/json", strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("设置命令菜单失败: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Telegram API 返回错误 (HTTP %d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		OK          bool   `json:"ok"`
		Result      bool   `json:"result"`
		Description string `json:"description"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
	}

	if !result.OK {
		return fmt.Errorf("Telegram API 返回失败: %s", result.Description)
	}

	logger.Println("Telegram Bot 命令菜单设置成功")
	return nil
}

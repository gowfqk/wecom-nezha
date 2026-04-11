package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"time"
)

type RequestBody struct {
	Sendkey  string    `json:"sendkey"`
	Msg      string    `json:"msg"`
	MsgType  string    `json:"msg_type"`
	ToUser   string    `json:"touser,omitempty"`
	AgentId  string    `json:"agentid,omitempty"`
	Text     *Msg      `json:"text,omitempty"`
	Markdown *Markdown `json:"markdown,omitempty"`
}

type Msg struct {
	Content string `json:"content"`
}

type Pic struct {
	MediaId string `json:"media_id"`
}

type Markdown struct {
	Content string `json:"content"`
}

type JsonData struct {
	ToUser                 string   `json:"touser"`
	AgentId                string   `json:"agentid"`
	MsgType                string   `json:"msgtype"`
	DuplicateCheckInterval int      `json:"duplicate_check_interval"`
	Text                   Msg      `json:"text"`
	Image                  Pic      `json:"image"`
	Markdown               Markdown `json:"markdown"`
}

type MailRequestBody struct {
	Sendkey       string           `json:"sendkey"`
	To            MailRecipient    `json:"to"`
	Cc            MailRecipient    `json:"cc,omitempty"`
	Bcc           MailRecipient    `json:"bcc,omitempty"`
	Subject       string           `json:"subject"`
	Content       string           `json:"content"`
	ContentType   string           `json:"content_type,omitempty"`
	AttachmentList []MailAttachment `json:"attachment_list,omitempty"`
	EnableIdTrans uint32           `json:"enable_id_trans,omitempty"`
}

type MailRecipient struct {
	Emails  []string `json:"emails,omitempty"`
	Userids []string `json:"userids,omitempty"`
}

type MailAttachment struct {
	FileName string `json:"file_name"`
	Content  string `json:"content"`
}

// 企微回调消息体
type WecomCallbackRequest struct {
	Signature string `json:"signature"`
	Timestamp string `json:"timestamp"`
	Nonce    string `json:"nonce"`
	Echostr  string `json:"echostr,omitempty"`
	Msg      string `json:"msg,omitempty"`
}

// 企微回调解密后的消息
type WecomCallbackMessage struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int      `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgId        int64    `xml:"MsgId"`
	AgentID      string   `xml:"AgentID"`
	Encrypt      string   `xml:"Encrypt"` // 加密模式下的消息内容
}

// FlexibleInt64 兼容 int64 和 string 类型的 JSON 字段
type FlexibleInt64 int64

func (f *FlexibleInt64) UnmarshalJSON(data []byte) error {
	// 尝试直接解析为数字
	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		*f = FlexibleInt64(n)
		return nil
	}
	// 尝试解析为字符串
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("FlexibleInt64: 无法解析为 int64 或 string: %s", string(data))
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		*f = FlexibleInt64(n)
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		*f = FlexibleInt64(t.Unix())
		return nil
	}
	return fmt.Errorf("FlexibleInt64: 无法将字符串 %q 解析为 int64", s)
}

// NezhaServer 服务器信息
type NezhaServer struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Tag        string `json:"note"`         // 哪吒 Server 模型的 note 字段（标签）
	LastActive   FlexibleInt64 `json:"last_active"`
	ValidIP      string        `json:"valid_ip"`
	Note         string        `json:"public_note"`
	Online       bool          `json:"online"`
	Host struct {
		Platform        string   `json:"platform"`
		PlatformVersion string   `json:"platform_version"`
		Arch            string   `json:"arch"`
		CPU             []string `json:"cpu"`
		MemTotal        uint64   `json:"mem_total"`
		DiskTotal       uint64   `json:"disk_total"`
		SwapTotal       uint64   `json:"swap_total"`
		Version         string   `json:"version"`
		BootTime        uint64   `json:"boot_time"`
		Virtualization  string   `json:"virtualization"`
	} `json:"host"`
	State struct {
		CPU           float64 `json:"cpu"`
		MemUsed       uint64  `json:"mem_used"`
		DiskUsed      uint64  `json:"disk_used"`
		SwapUsed      uint64  `json:"swap_used"`
		NetInSpeed    float64 `json:"net_in_speed"`
		NetOutSpeed   float64 `json:"net_out_speed"`
		NetInTransfer uint64  `json:"net_in_transfer"`
		NetOutTransfer uint64 `json:"net_out_transfer"`
		Load1         float64 `json:"load_1"`
		Load5         float64 `json:"load_5"`
		Load15        float64 `json:"load_15"`
		Uptime        uint64  `json:"uptime"`
		TCPConnCount  uint64  `json:"tcp_conn_count"`
		UDPConnCount  uint64  `json:"udp_conn_count"`
		ProcessCount  uint64  `json:"process_count"`
		GPU           []interface{} `json:"gpu"`
		Temperatures  []interface{} `json:"temperatures"`
	} `json:"state"`
}

// Nezha API 响应
type NezhaAPIResponse struct {
	Success bool        `json:"success"`
	Error   string     `json:"error"`
	Data    interface{} `json:"data"`
}

// Nezha 登录响应
type NezhaLoginResponse struct {
	Success bool           `json:"success"`
	Error   string         `json:"error"`
	Data    NezhaLoginData `json:"data"`
}

type NezhaLoginData struct {
	Token  string `json:"token"`
	Expire string `json:"expire"`
}
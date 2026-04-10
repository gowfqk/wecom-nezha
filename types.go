package main

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
	ToUserName   string
	FromUserName string
	CreateTime   int
	MsgType      string
	Content      string
	MsgId        int64
	AgentID      string
}

// Nezha API 响应
type NezhaServer struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Tag        string `json:"tag"`
	Note       string `json:"note,omitempty"`
	PublicNote string `json:"public_note,omitempty"`
	LastActive int64  `json:"last_active"`
	ValidIP    string `json:"valid_ip"`
	Online     bool   `json:"online"`
	Host       struct {
		Platform        string   `json:"Platform"`
		PlatformVersion string   `json:"PlatformVersion"`
		CPU            []string `json:"CPU"`
		MemTotal       uint64   `json:"MemTotal"`
		DiskTotal      uint64   `json:"DiskTotal"`
	} `json:"host"`
	Status struct {
		CPU         float64 `json:"CPU"`
		MemUsed     uint64  `json:"MemUsed"`
		DiskUsed    uint64  `json:"DiskUsed"`
		NetInSpeed  float64 `json:"NetInSpeed"`
		NetOutSpeed float64 `json:"NetOutSpeed"`
		Load1       float64 `json:"Load1"`
		Load5       float64 `json:"Load5"`
		Load15      float64 `json:"Load15"`
		Uptime      uint64  `json:"Uptime"`
	} `json:"status"`
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
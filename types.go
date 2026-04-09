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
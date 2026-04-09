package main

import (
	"context"
	"net/http"
	"sync"
	"time"
)

var Sendkey = GetEnvDefault("SENDKEY", "set_a_sendkey")
var WecomCid = GetEnvDefault("WECOM_CID", "企业微信公司ID")
var WecomSecret = GetEnvDefault("WECOM_SECRET", "企业微信应用Secret")
var WecomAid = GetEnvDefault("WECOM_AID", "企业微信应用ID")
var WecomToUid = GetEnvDefault("WECOM_TOUID", "@all")
var CacheType = GetEnvDefault("CACHE_TYPE", "none")
var RedisStat = GetEnvDefault("REDIS_STAT", "OFF")
var RedisAddr = GetEnvDefault("REDIS_ADDR", "localhost:6379")
var RedisPassword = GetEnvDefault("REDIS_PASSWORD", "")
var MailFooterUrl = GetEnvDefault("MAIL_FOOTER_URL", "")

var ctx = context.Background()
var httpClient = &http.Client{Timeout: 10 * time.Second}
var serverReadTimeout = 15 * time.Second
var serverWriteTimeout = 15 * time.Second
var serverIdleTimeout = 60 * time.Second

type MemoryCache struct {
	token      string
	expireTime time.Time
}

var memoryCache MemoryCache
var cacheMutex sync.RWMutex

var GetTokenApi = "https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s"
var SendMessageApi = "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=%s"
var UploadMediaApi = "https://qyapi.weixin.qq.com/cgi-bin/media/upload?access_token=%s&type=%s"
var MailComposeSendApi = "https://qyapi.weixin.qq.com/cgi-bin/exmail/app/compose_send?access_token=%s"

const RedisTokenKey = "access_token"

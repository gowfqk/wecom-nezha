package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

func GetRemoteToken(corpId, appSecret string) string {
	getTokenUrl := fmt.Sprintf(GetTokenApi, corpId, appSecret)
	log.Printf("getTokenUrl ==> %s", strings.Replace(getTokenUrl, appSecret, "***", 1))
	resp, err := httpClient.Get(getTokenUrl)
	if err != nil {
		log.Println(err)
		return ""
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		return ""
	}
	tokenResponse := ParseJson(string(respData))
	log.Println("企业微信获取access_token接口返回==>", tokenResponse)
	accessToken, ok := tokenResponse[RedisTokenKey].(string)
	if !ok || accessToken == "" {
		log.Println("企业微信获取access_token失败: missing access_token")
		return ""
	}
	if CacheType == "redis" && RedisStat == "ON" {
		set, err := RedisClient().SetNX(ctx, RedisTokenKey, accessToken, 7000*time.Second).Result()
		log.Println(set)
		if err != nil {
			log.Println(err)
		}
	} else if CacheType == "memory" {
		cacheMutex.Lock()
		memoryCache = MemoryCache{token: accessToken, expireTime: time.Now().Add(7000 * time.Second)}
		cacheMutex.Unlock()
	}
	return accessToken
}

func RedisClient() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: RedisAddr, Password: RedisPassword, DB: 0})
}

func PostMsg(postData JsonData, postUrl string) string {
	postJson, _ := json.Marshal(postData)
	msgReq, err := http.NewRequest("POST", postUrl, bytes.NewBuffer(postJson))
	if err != nil {
		log.Println(err)
		return `{"errcode":500,"errmsg":"create request failed"}`
	}
	msgReq.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(msgReq)
	if err != nil {
		log.Println("企业微信发送应用消息接口报错==>", err)
		return `{"errcode":500,"errmsg":"upstream request failed"}`
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取企业微信响应失败==>", err)
		return `{"errcode":500,"errmsg":"read upstream response failed"}`
	}
	log.Println("企业微信发送应用消息接口返回==>", ParseJson(string(body)))
	return string(body)
}

func UploadMedia(msgType string, req *http.Request, accessToken string) (string, float64) {
	_ = req.ParseMultipartForm(2 << 20)
	imgFile, imgHeader, err := req.FormFile("media")
	if err != nil {
		log.Println("图片文件出错==>", err)
		return "", 400
	}
	defer imgFile.Close()
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)
	createFormFile, err := writer.CreateFormFile("media", imgHeader.Filename)
	if err != nil {
		log.Println("创建 multipart 文件失败==>", err)
		return "", 500
	}
	readAll, err := io.ReadAll(imgFile)
	if err != nil {
		log.Println("读取图片文件失败==>", err)
		return "", 500
	}
	_, _ = createFormFile.Write(readAll)
	_ = writer.Close()
	uploadMediaUrl := fmt.Sprintf(UploadMediaApi, accessToken, msgType)
	newRequest, err := http.NewRequest("POST", uploadMediaUrl, buf)
	if err != nil {
		log.Println("创建上传请求失败==>", err)
		return "", 500
	}
	newRequest.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := httpClient.Do(newRequest)
	if err != nil {
		log.Println("上传临时素材出错==>", err)
		return "", 500
	}
	defer resp.Body.Close()
	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取上传响应失败==>", err)
		return "", 500
	}
	mediaResp := ParseJson(string(respData))
	if mediaID, ok := mediaResp["media_id"].(string); ok && mediaID != "" {
		return mediaID, 0
	}
	return "", getErrorCode(mediaResp)
}

func ValidateToken(errcode interface{}) bool {
	if errcode == nil {
		return true
	}
	codeTyp := reflect.TypeOf(errcode)
	if codeTyp == nil || !codeTyp.Comparable() {
		return true
	}
	f, ok := errcode.(float64)
	if !ok {
		return true
	}
	if math.Abs(f-float64(42001)) < 1e-3 {
		if CacheType == "redis" && RedisStat == "ON" {
			RedisClient().Del(ctx, RedisTokenKey)
		} else if CacheType == "memory" {
			cacheMutex.Lock()
			memoryCache = MemoryCache{}
			cacheMutex.Unlock()
		}
		return false
	}
	return true
}

func GetAccessToken() string {
	accessToken := ""
	if CacheType == "redis" && RedisStat == "ON" {
		value, err := RedisClient().Get(ctx, RedisTokenKey).Result()
		if err != nil && err != redis.Nil {
			log.Println("从redis获取token失败==>", err)
		}
		accessToken = value
	} else if CacheType == "memory" {
		cacheMutex.RLock()
		if !memoryCache.expireTime.IsZero() && time.Now().Before(memoryCache.expireTime) {
			accessToken = memoryCache.token
		}
		cacheMutex.RUnlock()
	}
	if accessToken == "" {
		accessToken = GetRemoteToken(WecomCid, WecomSecret)
	}
	return accessToken
}

func SendMailMessage(accessToken string, postData interface{}) string {
	postJson, _ := json.Marshal(postData)
	sendMailUrl := fmt.Sprintf(MailComposeSendApi, accessToken)
	msgReq, err := http.NewRequest("POST", sendMailUrl, bytes.NewBuffer(postJson))
	if err != nil {
		log.Println("创建邮件请求失败:", err)
		return `{"errcode":500,"errmsg":"create request failed"}`
	}
	msgReq.Header.Set("Content-Type", "application/json")
	resp, err := httpClient.Do(msgReq)
	if err != nil {
		log.Println("发送邮件失败==>", err)
		return `{"errcode":500,"errmsg":"upstream request failed"}`
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("读取邮件响应失败==>", err)
		return `{"errcode":500,"errmsg":"read upstream response failed"}`
	}
	return string(body)
}

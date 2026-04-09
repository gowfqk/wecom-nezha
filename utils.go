package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

func GetEnvDefault(key, defVal string) string {
	val, ex := os.LookupEnv(key)
	if !ex {
		return defVal
	}
	return val
}

func ParseJson(jsonStr string) map[string]interface{} {
	var wecomResponse map[string]interface{}
	if jsonStr != "" {
		err := json.Unmarshal([]byte(jsonStr), &wecomResponse)
		if err != nil {
			log.Println("生成json字符串错误")
		}
	}
	return wecomResponse
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 6 {
		return "***"
	}
	return s[:3] + strings.Repeat("*", len(s)-6) + s[len(s)-3:]
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func recoverMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				writeJSON(w, http.StatusInternalServerError, `{"errcode":50000,"errmsg":"internal server error"}`)
			}
		}()
		next(w, r)
	}
}

func requirePost(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, `{"errcode":405,"errmsg":"method not allowed"}`)
		return false
	}
	return true
}

func getErrorCode(m map[string]interface{}) float64 {
	if m == nil {
		return 0
	}
	v, ok := m["errcode"]
	if !ok || v == nil {
		return 0
	}
	if f, ok := v.(float64); ok {
		return f
	}
	return 0
}

func normalizeAppMsgType(msgType string) string {
	msgType = strings.TrimSpace(strings.ToLower(msgType))
	if msgType == "" {
		return "text"
	}
	return msgType
}

func validateMailRequestBody(requestBody *MailRequestBody) (int, string) {
	if len(requestBody.To.Emails) == 0 && len(requestBody.To.Userids) == 0 {
		return http.StatusBadRequest, `{"errcode":40010,"errmsg":"to.emails or to.userids is required"}`
	}
	requestBody.Subject = strings.TrimSpace(requestBody.Subject)
	if requestBody.Subject == "" {
		return http.StatusBadRequest, `{"errcode":40011,"errmsg":"subject is required"}`
	}
	requestBody.Content = strings.TrimSpace(requestBody.Content)
	if requestBody.Content == "" {
		return http.StatusBadRequest, `{"errcode":44004,"errmsg":"content is required"}`
	}
	return 0, ""
}
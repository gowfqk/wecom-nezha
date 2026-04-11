package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
)

// extractIPFromName 从服务器名称中提取IP（格式如 "ecs(10.0.0.1)"）
func extractIPFromName(name string) string {
	re := regexp.MustCompile(`\((\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\)`)
	matches := re.FindStringSubmatch(name)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// summarizeNote 解析 public_note，JSON 格式显示摘要，否则原样返回
func summarizeNote(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	// 尝试解析为 JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(note), &data); err != nil {
		return note // 非 JSON，原样返回
	}
	var parts []string
	// 提取 billingDataMod
	if billing, ok := data["billingDataMod"].(map[string]interface{}); ok {
		if cycle, ok := billing["cycle"].(string); ok && cycle != "" {
			parts = append(parts, cycle)
		}
		if amount, ok := billing["amount"].(string); ok && amount != "" {
			parts = append(parts, "¥"+amount)
		}
	}
	// 提取 planDataMod
	if plan, ok := data["planDataMod"].(map[string]interface{}); ok {
		if bw, ok := plan["bandwidth"].(string); ok && bw != "" {
			parts = append(parts, bw+"Mbps")
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " / ")
	}
	return "有备注"
}

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
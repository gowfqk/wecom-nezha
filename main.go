package main

import (
	"log"
	"net/http"
	"os"
)

var logger *log.Logger

func init() {
	logger = log.New(os.Stdout, "[wecom-nezha] ", log.LstdFlags|log.Lshortfile)
}

func main() {
	logger.Println("服务启动，监听端口 8080")
	
	http.HandleFunc("/wecomchan", recoverMiddleware(wecomChan))
	logger.Println("注册处理器: /wecomchan")
	
	http.HandleFunc("/mail", recoverMiddleware(mailHandler))
	logger.Println("注册处理器: /mail")
	
	http.HandleFunc("/callback", recoverMiddleware(WecomCallbackHandler))
	logger.Println("注册处理器: /callback")
	
	http.HandleFunc("/healthz", recoverMiddleware(healthz))
	logger.Println("注册处理器: /healthz")
	
	http.HandleFunc("/readyz", recoverMiddleware(readyz))
	logger.Println("注册处理器: /readyz")
	
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}
	
	logger.Println("开始监听...")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal(err)
	}
}

package main

import (
	"log"
	"net/http"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	http.HandleFunc("/wecomchan", recoverMiddleware(wecomChan))
	http.HandleFunc("/mail", recoverMiddleware(mailHandler))
	http.HandleFunc("/healthz", recoverMiddleware(healthz))
	http.HandleFunc("/readyz", recoverMiddleware(readyz))
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}
	log.Fatal(server.ListenAndServe())
}

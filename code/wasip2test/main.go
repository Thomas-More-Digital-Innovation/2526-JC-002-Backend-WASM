package main

import (
	"fmt"
	"net/http"

	_ "github.com/ydnar/wasi-http-go/wasihttp"
)

func init() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		msg := fmt.Sprintf("hello from wasi-http handler, method=%s path=%s\n", r.Method, r.URL.Path)
		_, _ = w.Write([]byte(msg))
	})
}

func main() {}

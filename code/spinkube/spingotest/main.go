package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	spinhttp "github.com/spinframework/spin-go-sdk/v2/http"
	"github.com/spinframework/spin-go-sdk/v2/pg"
)

func requestValues(r *http.Request) url.Values {
	values := url.Values{}
	if r.URL != nil {
		return r.URL.Query()
	}
	if r.RequestURI == "" {
		return values
	}
	parsedURI, err := url.ParseRequestURI(r.RequestURI)
	if err == nil {
		return parsedURI.Query()
	}
	return values
}

func init() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Home")
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "OK")
	})

	mux.HandleFunc("/api/hello", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Hello API")
	})

	mux.HandleFunc("/weather", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		values := requestValues(r)

		lat := values.Get("lat")
		lon := values.Get("lon")
		if lat == "" {
			lat = "51.2194"
		}
		if lon == "" {
			lon = "4.4025"
		}

		apiURL := fmt.Sprintf(
			"https://api.open-meteo.com/v1/forecast?latitude=%s&longitude=%s&current=temperature_2m,relative_humidity_2m,wind_speed_10m",
			url.QueryEscape(lat),
			url.QueryEscape(lon),
		)

		resp, err := spinhttp.Get(apiURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("weather api request failed: %v", err), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			http.Error(w, "failed to read weather api response", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(body)
	})

	mux.HandleFunc("/db-test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		values := requestValues(r)
		dsn := values.Get("dsn")
		if dsn == "" {
			dsn = os.Getenv("PG_DSN")
		}

		if dsn == "" {
			http.Error(w, "missing postgres dsn: set PG_DSN env var or pass ?dsn=...", http.StatusBadRequest)
			return
		}

		db := pg.Open(dsn)
		defer db.Close()

		var one int
		err := db.QueryRow("SELECT 1").Scan(&one)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":      true,
			"result":  one,
			"message": "postgres connection successful",
		})
	})

	spinhttp.Handle(mux.ServeHTTP)
}

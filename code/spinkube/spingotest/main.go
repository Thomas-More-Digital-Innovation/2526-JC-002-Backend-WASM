package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	spinhttp "github.com/spinframework/spin-go-sdk/v2/http"
)

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

		values := url.Values{}
		if r.URL != nil {
			values = r.URL.Query()
		} else if r.RequestURI != "" {
			parsedURI, err := url.ParseRequestURI(r.RequestURI)
			if err == nil {
				values = parsedURI.Query()
			}
		}

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

	spinhttp.Handle(mux.ServeHTTP)
}

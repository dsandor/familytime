// Family Time — family-friendly screen-time rules for UniFi gateways.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"familytime/internal/server"
	"familytime/internal/store"
	"familytime/internal/unifi"
	"familytime/web"
)

// loadDotEnv applies KEY=VALUE lines from ./.env without overriding real
// environment variables. Lets `familytime` pick up UNIFI_API_KEY for setup.
func loadDotEnv() {
	raw, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && os.Getenv(strings.TrimSpace(k)) == "" {
			os.Setenv(strings.TrimSpace(k), unquoteEnvValue(strings.TrimSpace(v)))
		}
	}
}

// unquoteEnvValue strips one layer of matching single or double quotes, so
// both KEY=value and KEY="value" / KEY='value' .env styles work.
func unquoteEnvValue(v string) string {
	if len(v) >= 2 {
		first, last := v[0], v[len(v)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

func main() {
	loadDotEnv()
	defaultData := ""
	if dir, err := os.UserConfigDir(); err == nil {
		defaultData = filepath.Join(dir, "familytime", "familytime.json")
	}
	port := flag.Int("port", 8080, "port for the web UI")
	data := flag.String("data", defaultData, "path to the familytime data file")
	flag.Parse()
	if *data == "" {
		log.Fatal("familytime: could not determine a config directory; pass --data")
	}

	st, err := store.Load(*data)
	if err != nil {
		log.Fatal(err)
	}
	srv := server.New(st, func(host, apiKey, fp string) server.UnifiAPI {
		return unifi.New(host, apiKey, fp)
	}, web.Static())
	srv.SetAdvertisedPort(*port)

	go srv.RunJanitor(context.Background(), 5*time.Minute)

	fmt.Printf("🛡️  Family Time is running — open http://localhost:%d\n", *port)
	fmt.Printf("   data file: %s\n", *data)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), srv.Handler()))
}

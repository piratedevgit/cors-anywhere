package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"golang.org/x/time/rate"
)

var (
	originBlacklist []string
	originWhitelist []string
	limiter         *rate.Limiter
)

func main() {
	// Listen on a specific host and port
	host := os.Getenv("HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = "8080"
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		log.Fatal("Invalid PORT value. Must be a valid integer.")
	}

	// Parse the origin blacklist and whitelist from environment variables
	originBlacklist = parseEnvList(os.Getenv("CORSANYWHERE_BLACKLIST"))
	originWhitelist = parseEnvList(os.Getenv("CORSANYWHERE_WHITELIST"))

	// Set up the rate limiter
	rateLimitStr := os.Getenv("RATE_LIMIT")
	if rateLimitStr == "" {
		rateLimitStr = "10" // Default rate limit: 10 requests per second
	}

	rateLimit, err := strconv.ParseFloat(rateLimitStr, 64)
	if err != nil {
		log.Fatal("Invalid RATE_LIMIT value. Must be a valid float.")
	}

	limiter = rate.NewLimiter(rate.Limit(rateLimit), 1)

	// Set up the HTTP server mux and routes
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleProxyRequest)

	// Start the server
	addr := host + ":" + strconv.Itoa(port)
	log.Println("Running CORS Anywhere on", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func parseEnvList(env string) []string {
	if env == "" {
		return nil
	}
	return strings.Split(env, ",")
}

func handleProxyRequest(w http.ResponseWriter, r *http.Request) {
	// Limit the rate of incoming requests
	if !limiter.Allow() {
		http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
		return
	}

	// Modify the request headers and handle CORS
	modifyRequestHeaders(r)

	// Create a forward request to the original target URL
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy the response headers
	copyResponseHeaders(w, resp)

	// Copy the response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func modifyRequestHeaders(r *http.Request) {
	// Modify the request headers as needed
	r.Header.Set("origin", r.Header.Get("Origin"))
	r.Header.Set("x-requested-with", r.Header.Get("X-Requested-With"))
}

func copyResponseHeaders(w http.ResponseWriter, resp *http.Response) {
	// Copy the response headers to the client
	for header, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(header, value)
		}
	}

	// Handle CORS headers
	origin := resp.Header.Get("Access-Control-Allow-Origin")
	if origin == "" && len(originWhitelist) > 0 {
		// If the original response doesn't include Access-Control-Allow-Origin header,
		// but there is an origin whitelist, add the first origin from the whitelist
		// as the allowed origin.
		origin = originWhitelist[0]
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}
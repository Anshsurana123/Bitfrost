package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

// Context keys for passing request metadata to ModifyResponse
type contextKey string

const bodyCtxKey contextKey = "bifrost_body"
const companyCtxKey contextKey = "bifrost_company"

// --- Configuration & Constants ---
const (
	UpstreamURL        = "https://generativelanguage.googleapis.com"
	MaxBodySize        = 2 * 1024 * 1024 // 2MB
	AuditorTimeoutMs   = 1500
	CBMaxFailures      = 5
	CBCooldownSeconds  = 60
	ReplayWindowSecs   = 60
	SavingsPerCacheHit = 0.015
	SemanticThreshold  = 0.88
)

// --- IN-MEMORY KV STORE (Replaces Redis for Local Demo) ---
type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string]interface{}
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string]interface{})}
}

func (s *InMemoryStore) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	if !ok {
		return "", fmt.Errorf("redis: nil")
	}
	if str, ok := val.(string); ok {
		return str, nil
	}
	return fmt.Sprintf("%v", val), nil
}

func (s *InMemoryStore) GetBytes(key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	val, ok := s.data[key]
	if !ok {
		return nil, fmt.Errorf("redis: nil")
	}
	if b, ok := val.([]byte); ok {
		return b, nil
	}
	if str, ok := val.(string); ok {
		return []byte(str), nil
	}
	return nil, fmt.Errorf("invalid type")
}

func (s *InMemoryStore) Set(key string, value interface{}, expiration time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = value
	return nil
}

func (s *InMemoryStore) DecrBy(key string, decrement int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	val, ok := s.data[key]
	if !ok {
		s.data[key] = fmt.Sprintf("%d", 100-decrement) // Default starting score 100
		return nil
	}
	if str, isStr := val.(string); isStr {
		if intVal, err := strconv.ParseInt(str, 10, 64); err == nil {
			s.data[key] = fmt.Sprintf("%d", intVal-decrement)
		}
	}
	return nil
}

// --- Global Metrics Aggregator ---
type GlobalMetrics struct {
	mu             sync.RWMutex
	RequestCount   int64
	CacheHits      int64
	BlockedAttacks int64
	CurrentLatency int64
	TotalSavings   float64
}

var metrics = &GlobalMetrics{}

// --- Semantic Brain Store (MULTI-TENANT) ---
type SemanticEntry struct {
	CompanyID string
	Embedding []float32
	Response  []byte
}

var semanticStore []SemanticEntry
var semanticMu sync.RWMutex

// --- WebSocket Hub ---
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
}

func (h *WSHub) broadcast(message []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for client := range h.clients {
		if err := client.WriteMessage(websocket.TextMessage, message); err != nil {
			client.Close()
			delete(h.clients, client)
		}
	}
}

var wsHub = &WSHub{clients: make(map[*websocket.Conn]bool)}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	wsHub.mu.Lock()
	wsHub.clients[conn] = true
	wsHub.mu.Unlock()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			wsHub.mu.Lock()
			delete(wsHub.clients, conn)
			wsHub.mu.Unlock()
			break
		}
	}
}

func startBroadcastLoop() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		metrics.mu.RLock()
		payload := map[string]interface{}{
			"type": "METRIC",
			"payload": map[string]interface{}{
				"timestamp": time.Now().Format("15:04:05"),
				"latency":   metrics.CurrentLatency,
				"savings":   metrics.TotalSavings,
			},
		}
		metrics.mu.RUnlock()

		msg, _ := json.Marshal(payload)
		wsHub.broadcast(msg)
	}
}

func pushWSEvent(eventType string, payload interface{}) {
	msg, _ := json.Marshal(map[string]interface{}{
		"type":    eventType,
		"payload": payload,
	})
	wsHub.broadcast(msg)
}

// --- Infrastructure ---

type BufferPool struct{ pool *sync.Pool }

func (b *BufferPool) Get() []byte  { return b.pool.Get().([]byte) }
func (b *BufferPool) Put(buf []byte) {
	if cap(buf) == 32*1024 {
		buf = buf[:0]
		b.pool.Put(buf)
	}
}

type CircuitBreaker struct {
	failures    int32
	lastFailure int64
}

func (cb *CircuitBreaker) RecordFailure() {
	atomic.AddInt32(&cb.failures, 1)
	atomic.StoreInt64(&cb.lastFailure, time.Now().Unix())
}
func (cb *CircuitBreaker) RecordSuccess() { atomic.StoreInt32(&cb.failures, 0) }
func (cb *CircuitBreaker) IsOpen() bool {
	if atomic.LoadInt32(&cb.failures) >= CBMaxFailures {
		if time.Now().Unix()-atomic.LoadInt64(&cb.lastFailure) < CBCooldownSeconds {
			return true
		}
	}
	return false
}

// BifrostProxy is the core data plane
type BifrostProxy struct {
	reverseProxy   *httputil.ReverseProxy
	kvStore        *InMemoryStore
	ollamaURL      string
	ollamaAPIKey   string
	circuitBreaker *CircuitBreaker
}

type MCPRequest struct {
	Method string `json:"method"`
	Reason string `json:"reason"`
}

// --- Initialization ---

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	targetURL, _ := url.Parse(UpstreamURL)
	kvStore := NewInMemoryStore()

	go startBroadcastLoop()

	customTransport := &http.Transport{
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 1000,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
	}

	ollamaURL := os.Getenv("OLLAMA_URL")
	if ollamaURL == "" {
		ollamaURL = "https://api.ollama.ai/v1/generate"
	}

	proxy := &BifrostProxy{
		reverseProxy: &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(targetURL)
				pr.Out.Host = targetURL.Host
				pr.Out.Header.Del("Accept-Encoding") // Force uncompressed response from Gemini
				virtualKey := pr.In.Header.Get("X-Bifrost-Key")
				if virtualKey != "" {
					realKey, err := kvStore.Get("key_map:" + virtualKey)
					if err != nil {
						// Fallback to Supabase if Render restarted
						supabaseUrl := os.Getenv("SUPABASE_URL")
						supabaseKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")
						if supabaseUrl != "" && supabaseKey != "" {
							urlStr := fmt.Sprintf("%s/rest/v1/bifrost_keys?select=real_key,company_id,app_secret&virtual_key=eq.%s", supabaseUrl, virtualKey)
							req, _ := http.NewRequest("GET", urlStr, nil)
							req.Header.Set("apikey", supabaseKey)
							req.Header.Set("Authorization", "Bearer "+supabaseKey)
							if resp, err := http.DefaultClient.Do(req); err == nil && resp.StatusCode == 200 {
								var result []struct {
									RealKey   string `json:"real_key"`
									CompanyID string `json:"company_id"`
									AppSecret string `json:"app_secret"`
								}
								if json.NewDecoder(resp.Body).Decode(&result) == nil && len(result) > 0 {
									realKey = result[0].RealKey
									kvStore.Set("key_map:"+virtualKey, result[0].RealKey, 0)
									kvStore.Set("key_company:"+virtualKey, result[0].CompanyID, 0)
									kvStore.Set("app_secret:"+virtualKey, result[0].AppSecret, 0)
								}
								resp.Body.Close()
							}
						}
					}
					
					if realKey != "" {
						// Gemini authenticates via ?key= query parameter
						q := pr.Out.URL.Query()
						q.Set("key", realKey)
						pr.Out.URL.RawQuery = q.Encode()
					}
				}
			},
			ModifyResponse: func(resp *http.Response) error {
				if resp.StatusCode != http.StatusOK {
					return nil
				}
				reqBody, ok := resp.Request.Context().Value(bodyCtxKey).([]byte)
				if !ok || len(reqBody) == 0 {
					return nil
				}
				companyID, ok := resp.Request.Context().Value(companyCtxKey).(string)
				if !ok || companyID == "" {
					companyID = "default"
				}

				// Check if this company has caching enabled
				cacheEnabled, _ := kvStore.Get("cache_enabled:" + companyID)
				if cacheEnabled == "false" {
					return nil // Bypass cache storage
				}

				// Read upstream response
				respBody, err := io.ReadAll(resp.Body)
				if err != nil {
					return err
				}
				resp.Body.Close()

				// Put body back for the client
				resp.Body = io.NopCloser(bytes.NewBuffer(respBody))
				resp.ContentLength = int64(len(respBody))

				// Store in L1 Direct Hash Cache & L2 Semantic Cache
				go func(rBody string, pBody []byte, compID string) {
					hash := sha256.Sum256([]byte(rBody))
					hashStr := hex.EncodeToString(hash[:])
					
					var reqObj struct {
						Prompt string `json:"prompt"`
					}
					json.Unmarshal([]byte(rBody), &reqObj)
					promptText := reqObj.Prompt
					if promptText == "" {
						promptText = rBody
					}
					
					emb, err := getEmbedding(promptText)
					if err != nil || len(emb) == 0 {
						log.Printf("[CACHE ERROR] Aborting cache storage. Semantic Brain embedding failed: %v", err)
						return
					}

					supabaseUrl := os.Getenv("SUPABASE_URL")
					supabaseKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")

					if supabaseUrl != "" && supabaseKey != "" {
						urlStr := fmt.Sprintf("%s/rest/v1/bifrost_cache", supabaseUrl)
						payload := map[string]interface{}{
							"company_id":  compID,
							"prompt_text": promptText,
							"prompt_hash": hashStr,
							"embedding":   emb,
							"response":    string(pBody),
						}
						jsonPayload, _ := json.Marshal(payload)
						
						req, _ := http.NewRequest("POST", urlStr, bytes.NewBuffer(jsonPayload))
						req.Header.Set("apikey", supabaseKey)
						req.Header.Set("Authorization", "Bearer "+supabaseKey)
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Prefer", "resolution=ignore-duplicates")
						
						resp, err := http.DefaultClient.Do(req)
						if err != nil {
							log.Printf("[SUPABASE ERROR] Network error: %v", err)
						} else if resp.StatusCode <= 299 {
							log.Printf("[SUPABASE] Permanently stored response & embeddings for Tenant %s", compID)
						} else {
							errorBody, _ := io.ReadAll(resp.Body)
							log.Printf("[SUPABASE ERROR] Status: %d, Response: %s", resp.StatusCode, string(errorBody))
						}
						
						if resp != nil {
							resp.Body.Close()
						}
					} else {
						// Fallback local memory
						kvStore.Set("cache:direct:"+compID+":"+hashStr, pBody, 24*time.Hour)
						semanticMu.Lock()
						semanticStore = append(semanticStore, SemanticEntry{
							CompanyID: compID,
							Embedding: emb,
							Response:  pBody,
						})
						semanticMu.Unlock()
						log.Printf("[CACHE] Local semantic brain trained on new prompt (Tenant: %s)", compID)
					}
				}(string(reqBody), respBody, companyID)

				return nil
			},
			Transport:  customTransport,
			BufferPool: &BufferPool{pool: &sync.Pool{New: func() interface{} { return make([]byte, 32*1024) }}},
		},
		kvStore:        kvStore,
		ollamaURL:      ollamaURL,
		ollamaAPIKey:   os.Getenv("OLLAMA_API_KEY"),
		circuitBreaker: &CircuitBreaker{},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws/metrics", wsHandler)
	mux.HandleFunc("/api/keys/generate", proxy.handleKeyGenerate)
	mux.HandleFunc("/api/keys/rotate", proxy.handleKeyRotate)
	mux.HandleFunc("/api/settings/cache", proxy.handleSettings)
	mux.HandleFunc("/mcp", proxy.handleMCP)
	mux.HandleFunc("/", proxy.ServeHTTP)

	// Configure CORS for the Key Vault UI
	corsHandler := func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Device-ID, X-Timestamp, X-Bifrost-Key, X-Device-Fingerprint")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			h.ServeHTTP(w, r)
		})
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Starting BIFRÖST Sovereign Proxy on :%s (MULTI-TENANT MODE)", port)
	log.Fatal(http.ListenAndServe(":"+port, corsHandler(mux)))
}

// --- Key Vault Logic ---

func (p *BifrostProxy) handleKeyGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		CompanyID string `json:"company_id"`
		RealKey   string `json:"real_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if req.CompanyID == "" {
		req.CompanyID = "default"
	}

	randBytes := make([]byte, 16)
	rand.Read(randBytes)
	virtualKey := "bf-vk-" + hex.EncodeToString(randBytes)

	rand.Read(randBytes)
	appSecret := "sec-" + hex.EncodeToString(randBytes)

	// Bind key to specific company in memory
	p.kvStore.Set("key_map:"+virtualKey, req.RealKey, 0)
	p.kvStore.Set("key_company:"+virtualKey, req.CompanyID, 0)
	p.kvStore.Set("app_secret:"+virtualKey, appSecret, 0)

	supabaseUrl := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")

	if supabaseUrl != "" && supabaseKey != "" {
		urlStr := fmt.Sprintf("%s/rest/v1/bifrost_keys", supabaseUrl)
		payload := map[string]interface{}{
			"virtual_key": virtualKey,
			"company_id":  req.CompanyID,
			"real_key":    req.RealKey,
			"app_secret":  appSecret,
		}
		jsonPayload, _ := json.Marshal(payload)
		
		reqObj, _ := http.NewRequest("POST", urlStr, bytes.NewBuffer(jsonPayload))
		reqObj.Header.Set("apikey", supabaseKey)
		reqObj.Header.Set("Authorization", "Bearer "+supabaseKey)
		reqObj.Header.Set("Content-Type", "application/json")
		
		resp, err := http.DefaultClient.Do(reqObj)
		if err != nil {
			log.Printf("[SUPABASE ERROR] Failed to save key: %v", err)
		} else if resp != nil {
			resp.Body.Close()
		}
	}

	// Enable caching by default for new companies
	if _, err := p.kvStore.Get("cache_enabled:" + req.CompanyID); err != nil {
		p.kvStore.Set("cache_enabled:"+req.CompanyID, "true", 0)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"virtual_key": virtualKey,
		"app_secret":  appSecret,
		"company_id":  req.CompanyID,
	})
}

func (p *BifrostProxy) handleKeyRotate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		VirtualKey string `json:"virtual_key"`
		NewRealKey string `json:"new_real_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	if req.VirtualKey == "" || req.NewRealKey == "" {
		http.Error(w, `{"error":"virtual_key and new_real_key are required"}`, http.StatusBadRequest)
		return
	}

	// Update in-memory store
	p.kvStore.Set("key_map:"+req.VirtualKey, req.NewRealKey, 0)
	log.Printf("[KEY ROTATION] Virtual key %s rotated to new provider key", req.VirtualKey[:12]+"...")

	// Update in Supabase
	supabaseUrl := os.Getenv("SUPABASE_URL")
	supabaseKey := os.Getenv("SUPABASE_SERVICE_ROLE_KEY")

	if supabaseUrl != "" && supabaseKey != "" {
		urlStr := fmt.Sprintf("%s/rest/v1/bifrost_keys?virtual_key=eq.%s", supabaseUrl, req.VirtualKey)

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


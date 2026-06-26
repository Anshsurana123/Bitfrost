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

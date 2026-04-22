package gateway

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// APIKeyAuthProvider is a simple API key based authentication provider
type APIKeyAuthProvider struct {
	keys map[string]string // maps API key to user ID
	mu   sync.RWMutex
}

// NewAPIKeyAuthProvider creates a new API key based auth provider
func NewAPIKeyAuthProvider() *APIKeyAuthProvider {
	return &APIKeyAuthProvider{
		keys: make(map[string]string),
	}
}

// ValidateToken checks if an API key is valid
func (p *APIKeyAuthProvider) ValidateToken(token string) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	userID, ok := p.keys[token]
	if !ok {
		return "", fmt.Errorf("invalid API key")
	}

	return userID, nil
}

// AddKey adds a new API key with associated user ID
func (p *APIKeyAuthProvider) AddKey(apiKey, userID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.keys[apiKey] = userID
}

// RemoveKey removes an API key
func (p *APIKeyAuthProvider) RemoveKey(apiKey string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.keys, apiKey)
}

// GenerateKey generates a new API key from a user ID (simple hash-based)
func GenerateKey(userID string) string {
	hash := sha256.Sum256([]byte(userID + fmt.Sprintf("%d", time.Now().UnixNano())))
	return fmt.Sprintf("sk_%x", hash)[:32]
}

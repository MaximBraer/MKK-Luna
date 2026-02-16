package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type StoredResponse struct {
	Status      int               `json:"status"`
	Body        []byte            `json:"body"`
	ContentType string            `json:"content_type"`
	Headers     map[string]string `json:"headers,omitempty"`
	RequestHash string            `json:"request_hash"`
	CreatedAt   int64             `json:"created_at"`
}

type Store struct {
	client *redis.Client
}

func NewStore(client *redis.Client) *Store {
	return &Store{client: client}
}

func (s *Store) Get(ctx context.Context, key string) (*StoredResponse, bool, error) {
	if s == nil || s.client == nil {
		return nil, false, nil
	}
	raw, err := s.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var out StoredResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, false, err
	}
	return &out, true, nil
}

func (s *Store) Set(ctx context.Context, key string, ttl time.Duration, v StoredResponse) error {
	if s == nil || s.client == nil {
		return nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, raw, ttl).Err()
}

func BuildRequestHash(method, routePattern, contentType string, query url.Values, body []byte) string {
	contentType = strings.TrimSpace(strings.ToLower(strings.Split(contentType, ";")[0]))
	parts := []string{
		strings.ToUpper(strings.TrimSpace(method)),
		strings.TrimSpace(routePattern),
		contentType,
		canonicalQuery(query),
		canonicalJSON(body),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

func BuildRouteHash(routePattern string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(routePattern)))
	return hex.EncodeToString(sum[:8])
}

func canonicalQuery(v url.Values) string {
	if len(v) == 0 {
		return ""
	}
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		vals := append([]string(nil), v[k]...)
		sort.Strings(vals)
		for _, one := range vals {
			parts = append(parts, k+"="+one)
		}
	}
	return strings.Join(parts, "&")
}

func canonicalJSON(raw []byte) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return string(raw)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(raw)
	}
	return string(b)
}

func bytesTrimSpace(in []byte) []byte {
	return []byte(strings.TrimSpace(string(in)))
}

func IsCacheableStatus(status int) bool {
	if status >= 200 && status < 300 {
		return true
	}
	switch status {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict:
		return true
	default:
		return false
	}
}

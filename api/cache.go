package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Cache keeps JSON values with the time they were stored. Only the API's own
// response types go in it, so it never holds the secrets of upstream records.
type Cache interface {
	Get(key string) (value []byte, storedAt time.Time, ok bool, err error)
	Set(key string, value []byte) error
}

// wantsRefresh reports whether the client asked to bypass the cache.
func wantsRefresh(r *http.Request) bool {
	refresh, _ := strconv.ParseBool(r.URL.Query().Get("refresh"))
	return refresh
}

// cached returns the value of key if it is younger than ttl, and otherwise
// loads it, with concurrent requests sharing one load. When a load fails and
// allowStale is set, an expired value is returned instead of the error. A cache
// that fails only costs extra loads.
func cached[T any](s *Server, key string, ttl time.Duration, refresh, allowStale bool, load func() (T, error)) (T, error) {
	var zero T
	var stale *T
	if s.options.Cache != nil {
		value, storedAt, ok, err := s.options.Cache.Get(key)
		if err != nil {
			log.Printf("cache get %s: %v", key, err)
		}
		if ok {
			var entry T
			if err := json.Unmarshal(value, &entry); err != nil {
				log.Printf("cache entry %s is invalid: %v", key, err)
			} else {
				if !refresh && time.Since(storedAt) < ttl {
					return entry, nil
				}
				stale = &entry
			}
		}
	}
	result, err, _ := s.flight.Do(key, func() (any, error) {
		value, err := load()
		if err != nil {
			return nil, err
		}
		if s.options.Cache != nil {
			encoded, err := json.Marshal(value)
			if err == nil {
				err = s.options.Cache.Set(key, encoded)
			}
			if err != nil {
				log.Printf("cache set %s: %v", key, err)
			}
		}
		return value, nil
	})
	if err != nil {
		if allowStale && stale != nil {
			log.Printf("serving expired %s: %v", key, err)
			return *stale, nil
		}
		return zero, err
	}
	return result.(T), nil
}

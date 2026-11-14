package app

import (
	"reflect"
	"sync"
)

// Services is a type-keyed registry for synchronous calls between modules.
// Prefer events; use this only when the caller needs a result immediately.
type Services struct {
	mu sync.RWMutex
	m  map[reflect.Type]any
}

func NewServices() *Services { return &Services{m: map[reflect.Type]any{}} }

// Provide registers impl under the interface type T.
func Provide[T any](s *Services, impl T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[reflect.TypeFor[T]()] = impl
}

// Lookup returns the service registered as T. ok is false when the providing
// module is disabled; callers must then use a fallback.
func Lookup[T any](s *Services) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[reflect.TypeFor[T]()]
	if !ok {
		var zero T
		return zero, false
	}
	return v.(T), true
}

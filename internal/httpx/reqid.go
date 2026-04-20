package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

type reqIDKey struct{}

// RequestID middleware assigns X-Request-ID and stores it in the context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			id = hex.EncodeToString(buf)
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), reqIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(reqIDKey{}).(string); ok {
		return v
	}
	return ""
}

package httpui

import "net/http"

func requireIdempotencyKey(r *http.Request) error {
	if r.Header.Get("Idempotency-Key") == "" {
		return errMissingIdempotencyKey
	}
	return nil
}

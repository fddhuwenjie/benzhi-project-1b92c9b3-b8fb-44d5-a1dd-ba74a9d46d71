package httpui

import "errors"

var errMissingIdempotencyKey = errors.New("缺少 Idempotency-Key")

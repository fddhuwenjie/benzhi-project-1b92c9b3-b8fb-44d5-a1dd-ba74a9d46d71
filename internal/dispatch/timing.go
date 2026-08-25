package dispatch

import "time"

const (
	lowReceiveWindow    = 8 * time.Hour
	normalReceiveWindow = 4 * time.Hour
	highReceiveWindow   = time.Hour
)

func receiveWindow(risk string) time.Duration {
	switch risk {
	case "高":
		return highReceiveWindow
	case "低":
		return lowReceiveWindow
	default:
		return normalReceiveWindow
	}
}

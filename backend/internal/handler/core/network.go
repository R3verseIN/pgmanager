package core

import (
	"net"
	"net/http"
	"strings"
)

func ClientIP(r *http.Request) string {
	remoteIPStr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIPStr = r.RemoteAddr
	}

	remoteIP := net.ParseIP(remoteIPStr)

	if remoteIP == nil || (!remoteIP.IsPrivate() && !remoteIP.IsLoopback()) {
		return remoteIPStr
	}

	if cfip := r.Header.Get("CF-Connecting-IP"); cfip != "" {
		return strings.TrimSpace(cfip)
	}
	if trueClient := r.Header.Get("True-Client-IP"); trueClient != "" {
		return strings.TrimSpace(trueClient)
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		return strings.TrimSpace(ips[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	return remoteIPStr
}

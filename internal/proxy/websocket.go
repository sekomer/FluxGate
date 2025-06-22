package proxy

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func isWebSocketRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("Upgrade")) == "websocket" &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request, target string) error {
	// * Parse the target URL to extract host:port
	targetURL, err := url.Parse(target)
	if err != nil {
		return err
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	targetConn, err := dialer.Dial("tcp", targetURL.Host)
	if err != nil {
		return err
	}
	defer targetConn.Close()

	hj, ok := w.(http.Hijacker)
	if !ok {
		return http.ErrNotSupported
	}

	clientConn, _, err := hj.Hijack()
	if err != nil {
		return err
	}
	defer clientConn.Close()

	if err := r.Write(targetConn); err != nil {
		return err
	}

	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(targetConn, clientConn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(clientConn, targetConn)
		errChan <- err
	}()

	<-errChan
	return nil
}

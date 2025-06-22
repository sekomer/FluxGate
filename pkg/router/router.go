package router

import (
	"net/http"
	"strings"
	"sync"
)

type Route struct {
	Path        string
	ServiceName string
	Methods     []string
}

type Router struct {
	routes []Route
	mu     sync.RWMutex
}

func New() *Router {
	return &Router{
		routes: make([]Route, 0),
	}
}

func (r *Router) AddRoute(path, serviceName string, methods []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.routes = append(r.routes, Route{
		Path:        path,
		ServiceName: serviceName,
		Methods:     methods,
	})
}

func (r *Router) Match(req *http.Request) *Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, route := range r.routes {
		if r.matchPath(req.URL.Path, route.Path) && r.matchMethod(req.Method, route.Methods) {
			return &route
		}
	}

	return nil
}

func (r *Router) matchPath(requestPath, routePath string) bool {
	if strings.HasSuffix(routePath, "*") {
		base := strings.TrimSuffix(routePath, "*")

		if strings.HasSuffix(base, "/") {
			trimmed := strings.TrimSuffix(base, "/")
			return requestPath == trimmed || strings.HasPrefix(requestPath, base)
		}

		return strings.HasPrefix(requestPath, base)
	}

	trim := func(s string) string {
		if s == "/" {
			return s
		}
		return strings.TrimSuffix(s, "/")
	}

	return trim(requestPath) == trim(routePath)
}

func (r *Router) matchMethod(requestMethod string, allowedMethods []string) bool {
	if len(allowedMethods) == 0 {
		return true
	}

	for _, method := range allowedMethods {
		if strings.EqualFold(requestMethod, method) {
			return true
		}
	}

	return false
}

func (r *Router) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routes = make([]Route, 0)
}

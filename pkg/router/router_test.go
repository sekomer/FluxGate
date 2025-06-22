package router

import (
	"net/http/httptest"
	"testing"
)

func TestRouter(t *testing.T) {
	tests := []struct {
		name   string
		routes []struct {
			path, service string
			methods       []string
		}
		requestPath     string
		requestMethod   string
		expectedResult  bool
		expectedService string
	}{
		{
			name: "exact path match",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/users", "user-service", []string{"GET", "POST"}},
			},
			requestPath:     "/api/users",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "user-service",
		},
		{
			name: "wildcard path match",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/*", "api-service", nil},
			},
			requestPath:     "/api/users/123",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "api-service",
		},
		{
			name: "method mismatch",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/users", "user-service", []string{"POST"}},
			},
			requestPath:    "/api/users",
			requestMethod:  "GET",
			expectedResult: false,
		},
		{
			name: "no match",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/users", "user-service", nil},
			},
			requestPath:    "/api/products",
			requestMethod:  "GET",
			expectedResult: false,
		},
		{
			name: "trailing slash handling",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/users/", "user-service", nil},
			},
			requestPath:     "/api/users",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "user-service",
		},
		{
			name: "multiple routes - first match wins",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/*", "first-service", nil},
				{"/api/users", "second-service", nil},
			},
			requestPath:     "/api/users",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "first-service",
		},
		{
			name: "empty methods allows all",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/test", "test-service", nil},
			},
			requestPath:     "/api/test",
			requestMethod:   "DELETE",
			expectedResult:  true,
			expectedService: "test-service",
		},
		{
			name: "case insensitive method matching",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/api/test", "test-service", []string{"get", "post"}},
			},
			requestPath:     "/api/test",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "test-service",
		},
		{
			name: "root path wildcard",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/*", "catch-all", nil},
			},
			requestPath:     "/anything/goes/here",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "catch-all",
		},
		{
			name: "exact root path",
			routes: []struct {
				path, service string
				methods       []string
			}{
				{"/", "root-service", nil},
			},
			requestPath:     "/",
			requestMethod:   "GET",
			expectedResult:  true,
			expectedService: "root-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New()

			for _, route := range tt.routes {
				r.AddRoute(route.path, route.service, route.methods)
			}

			req := httptest.NewRequest(tt.requestMethod, tt.requestPath, nil)
			result := r.Match(req)

			if tt.expectedResult {
				if result == nil {
					t.Errorf("expected route match, got nil")
				} else if result.ServiceName != tt.expectedService {
					t.Errorf("expected service %s, got %s", tt.expectedService, result.ServiceName)
				}
			} else {
				if result != nil {
					t.Errorf("expected no match, got %+v", result)
				}
			}
		})
	}
}

func TestRouterConcurrency(t *testing.T) {
	r := New()
	done := make(chan bool)

	go func() {
		for i := 0; i < 100; i++ {
			r.AddRoute("/test", "service", nil)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			req := httptest.NewRequest("GET", "/test", nil)
			r.Match(req)
		}
		done <- true
	}()

	<-done
	<-done
}

func TestRouterClear(t *testing.T) {
	r := New()

	r.AddRoute("/api/*", "api-service", nil)
	r.AddRoute("/health", "health-service", nil)

	req := httptest.NewRequest("GET", "/api/test", nil)
	if result := r.Match(req); result == nil {
		t.Error("Expected route to exist before clear")
	}

	r.Clear()

	if result := r.Match(req); result != nil {
		t.Error("Expected no routes after clear")
	}
}

func TestPathMatching(t *testing.T) {
	tests := []struct {
		routePath   string
		requestPath string
		shouldMatch bool
	}{
		{"/api/*", "/api/users", true},
		{"/api/*", "/api/users/123", true},
		{"/api/*", "/api/", true},
		{"/api/*", "/api", true},
		{"/api/*", "/api2/users", false},
		{"/exact", "/exact", true},
		{"/exact", "/exact/", true},
		{"/exact/", "/exact", true},
		{"/exact/", "/exact/", true},
		{"/*", "/anything", true},
		{"/*", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.routePath+"->"+tt.requestPath, func(t *testing.T) {
			router := New()
			router.AddRoute(tt.routePath, "test-service", nil)

			req := httptest.NewRequest("GET", tt.requestPath, nil)
			result := router.Match(req)

			if tt.shouldMatch && result == nil {
				t.Errorf("Expected path %s to match route %s", tt.requestPath, tt.routePath)
			} else if !tt.shouldMatch && result != nil {
				t.Errorf("Expected path %s to NOT match route %s", tt.requestPath, tt.routePath)
			}
		})
	}
}

#!/usr/bin/env python3
import http.server
import socketserver
import json
import sys
from urllib.parse import urlparse

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8001

class APIHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        parsed_path = urlparse(self.path)
        
        # API endpoints
        if parsed_path.path.startswith('/api/'):
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('X-Backend-Port', str(PORT))
            self.end_headers()
            
            response = {
                'message': f'Hello from backend on port {PORT}!',
                'path': parsed_path.path,
                'backend_port': PORT,
                'headers': dict(self.headers)
            }
            
            self.wfile.write(json.dumps(response, indent=2).encode())
        
        # Health check endpoint
        elif parsed_path.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps({'status': 'healthy', 'port': PORT}).encode())
        
        # Serve files for other paths
        else:
            super().do_GET()

with socketserver.TCPServer(("", PORT), APIHandler) as httpd:
    print(f"Backend server running on port {PORT}")
    httpd.serve_forever()
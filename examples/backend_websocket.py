#!/usr/bin/env python3
import http.server
import socketserver
import json
import sys
import hashlib
import base64
import struct
from urllib.parse import urlparse

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 8001


class WebSocketHandler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        parsed_path = urlparse(self.path)

        # WebSocket upgrade request
        if (
            self.headers.get("Upgrade", "").lower() == "websocket"
            and "upgrade" in self.headers.get("Connection", "").lower()
        ):
            self.handle_websocket_upgrade()
            return

        # API endpoints
        if parsed_path.path.startswith("/api/"):
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("X-Backend-Port", str(PORT))
            self.end_headers()

            response = {
                "message": f"Hello from backend on port {PORT}!",
                "path": parsed_path.path,
                "backend_port": PORT,
                "headers": dict(self.headers),
            }

            self.wfile.write(json.dumps(response, indent=2).encode())

        # Health check endpoint
        elif parsed_path.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps({"status": "healthy", "port": PORT}).encode())

        # Serve files for other paths
        else:
            super().do_GET()

    def handle_websocket_upgrade(self):
        """Handle WebSocket upgrade handshake"""
        try:
            key = self.headers["Sec-WebSocket-Key"]
            magic = b"258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
            accept = base64.b64encode(
                hashlib.sha1(key.encode() + magic).digest()
            ).decode()

            self.send_response(101, "Switching Protocols")
            self.send_header("Upgrade", "websocket")
            self.send_header("Connection", "Upgrade")
            self.send_header("Sec-WebSocket-Accept", accept)
            self.end_headers()

            # Simple echo WebSocket server
            while True:
                try:
                    frame = self.read_websocket_frame()
                    if frame is None:  # Connection closed
                        break

                    # Echo the message back
                    echo_msg = f"Echo from port {PORT}: {frame.decode('utf-8', errors='ignore')}"
                    self.send_websocket_frame(echo_msg.encode())

                except Exception as e:
                    print(f"WebSocket error: {e}")
                    break

        except Exception as e:
            print(f"WebSocket upgrade error: {e}")
            self.send_error(400, "Bad WebSocket request")

    def read_websocket_frame(self):
        """Read a WebSocket frame (simplified)"""
        try:
            header = self.rfile.read(2)
            if len(header) < 2:
                return None

            byte1, byte2 = struct.unpack("!BB", header)
            opcode = byte1 & 0x0F
            masked = byte2 & 0x80
            payload_length = byte2 & 0x7F

            if payload_length == 126:
                payload_length = struct.unpack("!H", self.rfile.read(2))[0]
            elif payload_length == 127:
                payload_length = struct.unpack("!Q", self.rfile.read(8))[0]

            if masked:
                mask = self.rfile.read(4)
                payload = self.rfile.read(payload_length)
                payload = bytes(b ^ mask[i % 4] for i, b in enumerate(payload))
            else:
                payload = self.rfile.read(payload_length)

            return payload if opcode == 1 else None  # Only handle text frames

        except:
            return None

    def send_websocket_frame(self, data):
        """Send a WebSocket frame"""
        frame = bytearray()
        frame.append(0x81)  # FIN + text frame

        if len(data) < 126:
            frame.append(len(data))
        elif len(data) < 65536:
            frame.append(126)
            frame.extend(struct.pack("!H", len(data)))
        else:
            frame.append(127)
            frame.extend(struct.pack("!Q", len(data)))

        frame.extend(data)
        self.wfile.write(frame)
        self.wfile.flush()


with socketserver.TCPServer(("", PORT), WebSocketHandler) as httpd:
    print(f"Backend server with WebSocket support running on port {PORT}")
    httpd.serve_forever()

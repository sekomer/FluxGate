#!/bin/bash

# Generate a self-signed certificate for testing
openssl req -x509 -newkey rsa:4096 -nodes -keyout key.pem -out cert.pem -days 365 -subj "/C=US/ST=State/L=City/O=FluxGate/CN=localhost"

echo "Generated cert.pem and key.pem for testing"
echo "These are self-signed certificates for development only!"
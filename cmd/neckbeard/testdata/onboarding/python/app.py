# Onboarding fixture: discovers an existing connection; this does not test DB I/O.
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
DATABASE_URL = os.environ.get("DATABASE_URL")
class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b'ok' if self.path == "/healthz" else b'hello')
HTTPServer(('0.0.0.0', 8000), Handler).serve_forever()

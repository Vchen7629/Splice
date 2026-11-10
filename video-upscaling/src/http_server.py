import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from prometheus_client import CONTENT_TYPE_LATEST, generate_latest


class HTTPHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/health":
            body = json.dumps({"status": "Healthy"}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
        elif self.path == "/metrics":
            body = generate_latest()
            self.send_response(200)
            self.send_header("Content-Type", CONTENT_TYPE_LATEST)
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_response(404)
            self.end_headers()


def start_http_server(port: int) -> ThreadingHTTPServer:
    """Starts the http serve and exposes 2 endpoints /health (for gateway probing) and /metrics (prometheus metrics)"""
    server = ThreadingHTTPServer(("", port), HTTPHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    return server

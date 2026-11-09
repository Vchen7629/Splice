import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class HealthEnpointHandler(BaseHTTPRequestHandler):
    def do_GET(self) -> None:
        if self.path == "/health":
            body = json.dumps({"status": "Healthy"}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(body)
        else:
            self.send_response(404)
            self.end_headers()


def start_health_server(port: int) -> ThreadingHTTPServer:
    server = ThreadingHTTPServer(("", port), HealthEnpointHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()

    return server

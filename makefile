
dev:
	docker compose up -d
	docker compose wait nats-init
	until curl -sf http://localhost:8888/ > /dev/null; do echo "waiting for seaweedfs filer..."; sleep 1; done
	uv run -m backend.scene-detector.src.service & \
	(cd backend/transcoder-worker && go run ./cmd/main.go) & \
	(cd backend/video-recombiner && go run ./cmd/main.go) & \
	(cd backend/video-upload && go run ./cmd/main.go) & \
	(cd backend/video-status && go run ./cmd/main.go) & \
	(cd frontend && npm run dev) & \
	wait

reset:
	docker compose down -v
	pkill -f "backend.scene-detector" 2>/dev/null || true
	pkill -f "transcoder-worker" 2>/dev/null || true
	pkill -f "video-recombiner" 2>/dev/null || true
	pkill -f "video-upload" 2>/dev/null || true
	pkill -f "video-status" 2>/dev/null || true
	pkill -f "npm run dev" 2>/dev/null || true
	pkill -f "vite" 2>/dev/null || true
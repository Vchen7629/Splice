
dev:
	docker compose up -d
	docker compose wait nats-init
	until curl -sf http://localhost:8888/ > /dev/null; do echo "waiting for seaweedfs filer..."; sleep 1; done
	setsid bash -c 'cd backend/video-status && go run ./cmd/main.go' & echo $$! > /tmp/splice-video-status.pid
	until curl -s -o /dev/null http://localhost:8085/; do echo "waiting for video-status..."; sleep 1; done
	setsid bash -c 'cd backend/scene-detector && uv run -m src.service' & echo $$! > /tmp/splice-scene-detector.pid
	setsid bash -c 'cd backend/video-upscaling && PYTHONPATH=src uv run -m src.service' & echo $$! > /tmp/splice-video-upscaling.pid
	setsid bash -c 'cd backend/transcoder-worker && go run ./cmd/main.go' & echo $$! > /tmp/splice-transcoder-worker.pid
	setsid bash -c 'cd backend/video-recombiner && go run ./cmd/main.go' & echo $$! > /tmp/splice-video-recombiner.pid
	setsid bash -c 'cd backend/video-upload && go run ./cmd/main.go' & echo $$! > /tmp/splice-video-upload.pid
	setsid bash -c 'cd frontend && npm run dev' & echo $$! > /tmp/splice-frontend.pid
	wait

reset:
	docker compose down -v
	for pid_file in /tmp/splice-*.pid; do \
		if [ -f "$$pid_file" ]; then \
			pid=$$(cat $$pid_file); \
			kill -- -$$pid 2>/dev/null || true; \
			rm -f "$$pid_file"; \
		fi; \
	done

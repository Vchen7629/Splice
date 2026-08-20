
dev:
	docker compose up -d
	docker compose wait nats-init
	until curl -sf http://localhost:8888/ > /dev/null; do echo "waiting for seaweedfs filer..."; sleep 1; done
	setsid bash -c 'cd go_services/cmd/gateway && go run .' & echo $$! > /tmp/splice-gateway.pid
	until curl -s -o /dev/null http://localhost:8080/; do echo "waiting for gateway..."; sleep 1; done
	setsid bash -c 'cd python_services/scene-detector && uv run -m src.service' & echo $$! > /tmp/splice-scene-detector.pid
	setsid bash -c 'cd python_services/video-upscaling && PYTHONPATH=src uv run -m src.service' & echo $$! > /tmp/splice-video-upscaling.pid
	setsid bash -c 'cd go_services/cmd/transcoder && go run .' & echo $$! > /tmp/splice-transcoder.pid
	setsid bash -c 'cd go_services/cmd/recombiner && go run .' & echo $$! > /tmp/splice-recombiner.pid
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

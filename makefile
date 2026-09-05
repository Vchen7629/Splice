
dev:
	docker compose up -d
	docker compose wait nats-init
	until curl -sf http://localhost:8888/ > /dev/null; do echo "waiting for seaweedfs filer..."; sleep 1; done
	setsid bash -c 'echo $$BASHPID > /tmp/splice-gateway.pid; cd go_services/cmd/gateway && exec go run .' > /tmp/splice-gateway.log 2>&1 &
	gateway_ready=0; \
	for i in $$(seq 1 60); do \
		if curl -sf -o /dev/null http://localhost:8080/health; then gateway_ready=1; break; fi; \
		if ! kill -0 "$$(cat /tmp/splice-gateway.pid 2>/dev/null)" 2>/dev/null; then break; fi; \
		echo "waiting for gateway..."; sleep 1; \
	done; \
	if [ "$$gateway_ready" != "1" ]; then \
		echo "gateway failed to start, see /tmp/splice-gateway.log"; \
		cat /tmp/splice-gateway.log; \
		exit 1; \
	fi
	setsid bash -c 'echo $$BASHPID > /tmp/splice-scene-detector.pid; cd python_services/scene-detector && exec uv run -m src.service' > /tmp/splice-scene-detector.log 2>&1 &
	setsid bash -c 'echo $$BASHPID > /tmp/splice-video-upscaling.pid; cd python_services/video-upscaling && PYTHONPATH=src exec uv run -m src.service' > /tmp/splice-video-upscaling.log 2>&1 &
	setsid bash -c 'echo $$BASHPID > /tmp/splice-transcoder.pid; cd go_services/cmd/transcoder && exec go run .' > /tmp/splice-transcoder.log 2>&1 &
	setsid bash -c 'echo $$BASHPID > /tmp/splice-recombiner.pid; cd go_services/cmd/recombiner && exec go run .' > /tmp/splice-recombiner.log 2>&1 &
	setsid bash -c 'echo $$BASHPID > /tmp/splice-frontend.pid; cd frontend && exec npm run dev' > /tmp/splice-frontend.log 2>&1 &
	@echo "All services started in background. Logs: /tmp/splice-*.log (run 'make logs' to tail). Run 'make reset' to stop everything."

logs:
	tail -f /tmp/splice-*.log

reset:
	docker compose down -v
	for pid_file in /tmp/splice-*.pid; do \
		if [ -f "$$pid_file" ]; then \
			pid=$$(cat $$pid_file); \
			kill -- -$$pid 2>/dev/null || true; \
			rm -f "$$pid_file"; \
		fi; \
	done
	rm -f /tmp/splice-*.log
	for port in 8080 9090 9095 9098 9101 5173 5174; do \
		fuser -k $$port/tcp 2>/dev/null || true; \
	done

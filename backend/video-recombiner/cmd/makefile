test_all: integration unit

PKGS := . ../internal/handler/... ../internal/observability/... ../internal/service/... ../internal/storage/...

format: 
	go fmt ${PKGS} .
	go fmt ../internal/test/... .

lint:
	golangci-lint run ${PKGS}
	golangci-lint run ../internal/test/...

coverage:
	go test -tags=unit,integration ${PKGS} -v -coverprofile=coverage.out -covermode=atomic

integration:
	go test -tags integration ${PKGS}

unit:
	go test -tags unit ${PKGS}
	go test -race -tags unit ../internal/service/...

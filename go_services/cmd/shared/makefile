test_all: integration unit

PKGS := ../../internal/shared/...

format: 
	go fmt ${PKGS} .

lint:
	golangci-lint run ${PKGS}

coverage:
	go test -tags=unit,integration ${PKGS} -v -coverprofile=coverage.out -covermode=atomic

integration:
	go test -tags integration ${PKGS}

unit:
	go test -tags unit ${PKGS}
test_all: integration unit

PKGS := ./kv/... ./middleware/... ./storage/...

format: 
	go fmt ${PKGS} .
	go fmt ./test/... .

lint:
	golangci-lint run ${PKGS}
	golangci-lint run ./test/...

coverage:
	go test -tags=unit,integration ${PKGS} -v -coverprofile=coverage.out -covermode=atomic

integration:
	go test -tags integration ${PKGS}

unit:
	go test -tags unit ${PKGS}
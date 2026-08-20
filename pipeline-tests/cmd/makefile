PKGS := ../...

format:
	gofmt -l -w ..

lint:
	golangci-lint run --build-tags integration ${PKGS}

coverage:
	go test -tags integration ${PKGS} -v -coverprofile=coverage.out -covermode=atomic

test:
	go test -tags integration ${PKGS}

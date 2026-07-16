.PHONY: fmt vet test race build tidy check

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

race:
	go test -race ./...

build:
	go build ./...

tidy:
	go mod tidy
	git diff --exit-code -- go.mod go.sum

check: fmt tidy vet race build

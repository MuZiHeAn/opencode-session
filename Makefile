.PHONY: build test vet fmt

build:
	go build -trimpath -o opencode-session .

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

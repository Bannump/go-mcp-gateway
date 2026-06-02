.PHONY: build test lint run run-stdio docker-build docker-run coverage

build:
	go build ./cmd/server

test:
	go test ./... -race

lint:
	golangci-lint run

run:
	go run ./cmd/server

run-stdio:
	TRANSPORT=stdio go run ./cmd/server

docker-build:
	docker build -t go-mcp-gateway .

docker-run:
	docker run --env-file .env -p 8080:8080 go-mcp-gateway

coverage:
	go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out

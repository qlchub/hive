.DEFAULT: build

build: lint generate
	@go build ./...

test: generate
	@go test -short -cover -coverprofile=coverage.txt ./...

lint:
	@go tool staticcheck ./...
	@go tool golangci-lint run ./...

generate:
	@go tool mockery --config .mockery.yaml

benchmark:
	@go test -bench=. -benchmem

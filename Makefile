.PHONY: build test lint vet run docker clean

BIN := geokeep
CGO ?= 0

build:
	CGO_ENABLED=$(CGO) go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/geokeep

test:
	CGO_ENABLED=$(CGO) go test ./... -count=1

race:
	CGO_ENABLED=1 go test ./... -race -count=1

vet:
	CGO_ENABLED=$(CGO) go vet ./...

run: build
	GEOKEEP_SECRET?=$$(head -c32 /dev/urandom | base64 | tr -d '=+/' | head -c 32); \
	GEOKEEP_DATA_DIR=./data ./$(BIN) serve

docker:
	docker build -t geokeep:dev .

clean:
	rm -f $(BIN)
	rm -rf data

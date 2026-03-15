APP := frankie
PORT ?= 3593

.PHONY: run build fmt test clean

run:
	PORT=$(PORT) go run ./src

build:
	go build -o $(APP) ./src

fmt:
	gofmt -w ./src/*.go

test:
	go test ./...

clean:
	rm -f $(APP)

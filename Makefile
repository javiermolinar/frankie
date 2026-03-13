APP := frankie
PORT ?= 3593

.PHONY: run build fmt test clean

run:
	PORT=$(PORT) go run .

build:
	go build -o $(APP) .

fmt:
	gofmt -w main.go

test:
	go test ./...

clean:
	rm -f $(APP)

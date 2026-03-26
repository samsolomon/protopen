BINARY = velori

.PHONY: build test clean

build:
	go build -o $(BINARY) .

test:
	go test -v ./...

clean:
	rm -f $(BINARY)

.PHONY: build run stop clean test

build:
	go build -o xgate ./cmd/xgate

run: build
	sudo ./xgate serve

stop:
	@sudo pkill -SIGTERM xgate 2>/dev/null && echo "Stopped." || echo "Not running."

test:
	go test ./...

clean:
	rm -f xgate

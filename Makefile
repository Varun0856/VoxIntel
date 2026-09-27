.PHONY: build test test-race vet fmt run docker-build docker-run clean

build:
	go build -o voxintel .

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

run:
	go run .

docker-build:
	docker build -t voxintel .

docker-run:
	docker run -p 8080:8080 --env-file .env -v voxintel-data:/app/data voxintel

clean:
	rm -f voxintel

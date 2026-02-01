.PHONY: build run clean docker test deps fmt

# Binary name
BINARY=requestarr

# Build the application
build:
	CGO_ENABLED=1 go build -o $(BINARY) ./cmd/server

# Build for production (optimized)
build-prod:
	CGO_ENABLED=1 go build -ldflags="-s -w" -o $(BINARY) ./cmd/server

# Run the application
run: build
	./$(BINARY)

# Run with custom settings
run-dev:
	PORT=5000 DB_PATH=./requestarr.db ADMIN_PASSWORD=admin go run ./cmd/server

# Clean build artifacts
clean:
	rm -f $(BINARY)
	rm -f *.db
	rm -rf config/

# Build Docker image
docker:
	docker build -t requestarr .

# Run with Docker
docker-run:
	docker run -d \
		--name requestarr \
		-p 5000:5000 \
		-v $(PWD)/config:/config \
		-e ADMIN_PASSWORD=admin \
		requestarr

# Run with Docker Compose
compose-up:
	docker compose up -d

# Stop Docker Compose
compose-down:
	docker compose down

# View Docker logs
compose-logs:
	docker compose logs -f

# Download dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run tests
test:
	go test ./...

# System Design Playground: Go simulator + React UI
# Requires Go >= 1.23 and Node >= 22.12.

PORT ?= 8090
export ADDR := :$(PORT)

.PHONY: help install dev dev-backend dev-frontend build run test test-backend test-frontend lint bench docker-build docker-run up down logs

help:
	@echo "make install   install frontend dependencies"
	@echo "make dev       run API (:$(PORT)) and Vite dev server (:5173) together"
	@echo "make build     build the Go binary and the production frontend"
	@echo "make run       serve the built frontend from the Go binary on :$(PORT)"
	@echo "make test      run backend and frontend tests"
	@echo "make lint      gofmt check and staticcheck on the backend"
	@echo "make bench     engine benchmark (30 simulated seconds of a 1k rps system)"
	@echo "make up         build and run in the background on :$(PORT) (restarts automatically, keeps shared designs)"
	@echo "make down | logs   stop and remove it | follow its logs"
	@echo "make docker-build / docker-run   build the image / run it in the foreground"

install:
	cd frontend && npm install

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && BACKEND_URL=http://localhost:$(PORT) npm run dev

dev:
	@trap 'kill 0' INT TERM EXIT; \
	$(MAKE) dev-backend & $(MAKE) dev-frontend & wait

build:
	cd backend && go build -o bin/server ./cmd/server
	cd frontend && npm run build

run: build
	./backend/bin/server -static frontend/dist

test: test-backend test-frontend

test-backend:
	cd backend && go vet ./... && go test -count=1 ./...

test-frontend:
	cd frontend && npm run typecheck && npm test

lint:
	cd backend && test -z "$$(gofmt -l .)" && go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...

bench:
	cd backend && go test -run xxx -bench . -benchtime 5x ./internal/engine/

IMAGE ?= system-design-playground

docker-build:
	docker build -t $(IMAGE) .

docker-run: docker-build
	docker run --rm -p $(PORT):8090 -v sdp-data:/data $(IMAGE)

CONTAINER ?= system-design-playground

# run in the background, restarting after a reboot or crash, with shared designs kept in a named volume
up: docker-build
	-docker rm -f $(CONTAINER)
	docker run -d --name $(CONTAINER) --restart unless-stopped -p $(PORT):8090 -v sdp-data:/data $(IMAGE)
	@echo "Running at http://localhost:$(PORT)  (make logs to follow, make down to stop)"

down:
	docker rm -f $(CONTAINER)

logs:
	docker logs -f $(CONTAINER)

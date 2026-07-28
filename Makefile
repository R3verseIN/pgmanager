.PHONY: build test test-unit test-integration test-e2e lint fmt clean

build:
	cd backend && go build ./...

test:
	cd backend && go test ./... -count=1

test-v:
	cd backend && go test -v ./... -count=1

test-unit:
	cd backend && go test ./internal/handler/core/ ./internal/handler/testutil/ ./internal/platform/... -count=1

test-integration:
	cd backend && go test -tags integration ./internal/handler/... -count=1 -timeout 10m

test-e2e:
	docker compose -f docker-compose.test.yml -p pgtest up --build --abort-on-container-exit --exit-code-from e2e e2e
	docker compose -f docker-compose.test.yml -p pgtest down -v

lint:
	cd backend && go vet ./...
	cd backend && go vet -tags integration ./...

fmt:
	cd backend && gofmt -l .
	cd backend && gofmt -w .

clean:
	cd backend && go clean -testcache
	docker ps -a --filter "label=org.testcontainers" -q | xargs -r docker rm -f

SERVER := apps/server
COMPOSE := docker compose -f deploy/docker-compose.yml

.PHONY: up down server test lint migrate openapi web

up:        ## start postgres, redis, minio
	$(COMPOSE) up -d
down:
	$(COMPOSE) down
server:    ## run the API server
	cd $(SERVER) && go run ./cmd/server
migrate:   ## run migrations for enabled modules
	cd $(SERVER) && go run ./cmd/migrate
test:
	cd $(SERVER) && go test ./...
	npm test
lint:
	cd $(SERVER) && go vet ./... && golangci-lint run
	npm run lint
openapi:   ## regenerate api/openapi.yaml and packages/core types
	cd $(SERVER) && go run ./cmd/server openapi > api/openapi.yaml
	npm run gen:api -w packages/core
web:
	npm run dev:web

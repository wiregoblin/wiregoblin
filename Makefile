PROJECT := config/example.project.yaml
DOCKER ?= docker
GOLANGCI_LINT_VERSION ?= v2.10-alpine
GOLANGCI_LINT_IMAGE   ?= golangci/golangci-lint:$(GOLANGCI_LINT_VERSION)
DOCKER_IMAGE ?= wiregoblin/wiregoblin
DOCKER_TAG ?= cli-latest
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64,linux/arm/v7

.PHONY: run-http-example run-grpc-example run-local-stack-example run-goto-example run-error-handler-example run-workflow-block-example compose-up compose-down run docker-build docker-push

run-http-example:
	go run ./cmd/cli run -p $(PROJECT) http_example

run-grpc-example:
	go run ./cmd/cli run -vv -p $(PROJECT) grpc_example

run-local-stack-example:
	go run ./cmd/cli run -vv -p $(PROJECT) local_stack_example

run-goto-example:
	go run ./cmd/cli run -p $(PROJECT) goto_example

run-error-handler-example:
	go run ./cmd/cli run -p $(PROJECT) error_handler_example

run-workflow-block-example:
	go run ./cmd/cli run  -p $(PROJECT) workflow_block_example

compose-up:
	docker compose -f docker-compose.example.yaml up -d

compose-down:
	docker compose -f docker-compose.example.yaml down -v

run:
	go run ./cmd/cli run -p $(PROJECT) $(WORKFLOW)

lint-docker: ## Run golangci-lint inside Docker (no local install required)
	@echo "Running golangci-lint in Docker image $(GOLANGCI_LINT_IMAGE)"
	@$(DOCKER) run --rm \
		-e CGO_ENABLED=0 \
		-v $(shell pwd):/app \
		-w /app \
		$(GOLANGCI_LINT_IMAGE) \
		golangci-lint run ./...

docker-build: ## Build CLI Docker image for all configured platforms
	$(DOCKER) buildx build --platform $(DOCKER_PLATFORMS) -t $(DOCKER_IMAGE):$(DOCKER_TAG) .

docker-push: ## Build and push CLI Docker image for all configured platforms
	$(DOCKER) buildx build --platform $(DOCKER_PLATFORMS) -t $(DOCKER_IMAGE):$(DOCKER_TAG) --push .

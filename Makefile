PROJECT ?= config/example.project.yaml
WORKFLOW ?=

CLI_PACKAGE ?= ./cmd/wiregoblin-cli
CLI ?= go run $(CLI_PACKAGE)

DOCKER ?= docker
COMPOSE_FILE ?= docker-compose.example.yaml
COMPOSE ?= $(DOCKER) compose -f $(COMPOSE_FILE)

GOLANGCI_LINT_VERSION ?= v2.10-alpine
GOLANGCI_LINT_IMAGE ?= golangci/golangci-lint:$(GOLANGCI_LINT_VERSION)

DOCKER_IMAGE ?= wiregoblin/wiregoblin
DOCKER_TAG ?= cli-latest
DOCKER_PLATFORMS ?= linux/amd64,linux/arm64,linux/arm/v7
DOCKER_BUILD_ARGS ?= --platform $(DOCKER_PLATFORMS) -t $(DOCKER_IMAGE):$(DOCKER_TAG)

.PHONY: \
	help \
	compose-up \
	compose-down \
	run \
	run-http-example \
	run-http-post-example \
	run-http-example-json \
	run-grpc-example \
	run-local-stack-example \
	run-ai-summary-example \
	run-ai-success-summary-example \
	run-goto-example \
	run-workflow-timeout-example \
	run-retry-example \
	run-retry-exhaustion-example \
	run-foreach-example \
	run-foreach-range-example \
	run-postgres-transaction-example \
	run-parallel-example \
	run-transform-example \
	run-email-example \
	run-error-handler-example \
	run-continue-on-error-example \
	run-secret-variables-example \
	run-workflow-block-example \
	lint-docker \
	docker-build \
	docker-push

define run_workflow
	$(CLI) run $(1) -p $(PROJECT) $(2)
endef

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_-]+:.*## / {printf "%-32s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

run-http-example: ## Run the HTTP example workflow
	$(call run_workflow,,http_example)

run-http-post-example: ## Run the HTTP POST example workflow
	$(call run_workflow,,http_post_example)

run-http-example-json: ## Run the HTTP example workflow with JSON output
	$(call run_workflow,--json,http_example)

run-grpc-example: ## Run the gRPC example workflow
	$(call run_workflow,-vv,grpc_example)

run-local-stack-example: ## Run the local stack example workflow
	$(call run_workflow,-vv,local_stack_example)

run-ai-summary-example: ## Run a failing workflow and print automatic AI summary (requires local AI server)
	-$(call run_workflow,,error_handler_example)

run-ai-success-summary-example: ## Run a successful workflow and print AI summary (requires local AI server)
	$(call run_workflow,--ai-summary-success,http_example)

run-goto-example: ## Run the goto example workflow
	$(call run_workflow,,goto_example)

run-workflow-timeout-example: ## Run the workflow timeout example
	$(call run_workflow,,workflow_timeout_example)

run-retry-example: ## Run the retry example workflow
	$(call run_workflow,-vv,retry_example)

run-retry-exhaustion-example: ## Run the retry exhaustion example workflow
	$(call run_workflow,-vv,retry_exhaustion_example)

run-foreach-example: ## Run the foreach example workflow
	$(call run_workflow,-vvv,foreach_example)

run-foreach-range-example: ## Run the foreach range example workflow
	$(call run_workflow,-vv,foreach_range_example)

run-postgres-transaction-example: ## Run the Postgres transaction example workflow
	$(call run_workflow,-vv,postgres_transaction_example)

run-parallel-example: ## Run the parallel example workflow
	$(call run_workflow,-vv,parallel_example)

run-transform-example: ## Run the transform example workflow
	$(call run_workflow,-vvv,transform_example)

run-email-example: ## Run the email example workflow
	$(call run_workflow,-vv,email_example)

run-error-handler-example: ## Run the error handler example workflow
	$(call run_workflow,,error_handler_example)

run-continue-on-error-example: ## Run the continue-on-error example workflow
	$(call run_workflow,,continue_on_error_example)

run-secret-variables-example: ## Run the secret variables example workflow
	$(call run_workflow,,secret_variables_example)

run-workflow-block-example: ## Run the workflow block example
	$(call run_workflow,,workflow_block_example)

compose-up: ## Start the local example stack
	$(COMPOSE) up -d

compose-down: ## Stop the local example stack and remove volumes
	$(COMPOSE) down -v

run: ## Run an arbitrary workflow: make run WORKFLOW=http_example
	@test -n "$(WORKFLOW)" || (echo "WORKFLOW is required, e.g. make run WORKFLOW=http_example" && exit 1)
	$(call run_workflow,,$(WORKFLOW))

lint-docker: ## Run golangci-lint inside Docker (no local install required)
	@echo "Running golangci-lint in Docker image $(GOLANGCI_LINT_IMAGE)"
	@$(DOCKER) run --rm \
		-e CGO_ENABLED=0 \
		-v $(CURDIR):/app \
		-w /app \
		$(GOLANGCI_LINT_IMAGE) \
		golangci-lint run ./...

docker-build: ## Build the CLI Docker image for all configured platforms
	$(DOCKER) buildx build $(DOCKER_BUILD_ARGS) .

docker-push: ## Build and push the CLI Docker image for all configured platforms
	$(DOCKER) buildx build $(DOCKER_BUILD_ARGS) --push .

# 🧌 WireGoblin

Workflow runner for integration testing and automation. Define steps in YAML — call HTTP APIs, gRPC services, databases, run containers, assert results, and loop with goto.

## Install

```bash
go install github.com/wiregoblin/wiregoblin/cmd/cli@latest
mv "$(go env GOPATH)/bin/cli" "$(go env GOPATH)/bin/wiregoblin"
```

## Run

```bash
wiregoblin run <workflow_id>
wiregoblin run -p config/example.project.yaml http_example
```

If `-p` is omitted, WireGoblin looks for `wiregoblin.yaml` or `wiregoblin.yml` in the current directory.

Verbosity:

- default: live progress and final summary
- `-v`: add step outcome and timing
- `-vv`: add compact response summaries
- `-vvv`: add full request/response payloads
- `--json`: stream NDJSON events to stdout

## Config

```yaml
id: demo
name: Demo

constants:
  api_host: https://api.example.com

secrets:
  api_token: ${API_TOKEN}

workflows:
  http_example:
    name: HTTP Example
    blocks:
      get_user:
        type: http
        method: GET
        url: "$api_host/users/1"
        response_mapping:
          - key: user_id
            path: body.id

      notify:
        type: telegram
        token: "@api_token"
        chat_id: "123456789"
        message: "User @user_id has been created"
```

References:

- `@name` — runtime variable or secret
- `$name` — project or workflow constant
- inline interpolation is supported: `"Bearer @token"`, `"Hello, @user!"`

## Blocks

| Block | Description |
|-------|-------------|
| `http` | HTTP request |
| `grpc` | gRPC unary call via server reflection |
| `postgres` | SQL query |
| `redis` | Redis command |
| `openai` | OpenAI-compatible chat completion |
| `telegram` | Telegram message |
| `container` | Docker container job |
| `delay` | Pause execution |
| `setvars` | Assign variables |
| `assert` | Fail if condition is not met |
| `goto` | Conditional jump (loop support) |
| `workflow` | Run a nested workflow |

Notes:

- `http`, `openai`, and `telegram` accept optional `timeout_seconds`
- `container` runs `sh -c` inside the configured image; treat `command` as trusted input
- gRPC `bytes` fields use raw strings by default; prefix with `base64:` for binary values

## Docker

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  wiregoblin/wiregoblin:cli-latest \
  run http_example
```

Use `-p` if your config is not named `wiregoblin.yaml`:

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  wiregoblin/wiregoblin:cli-latest \
  run -p /workspace/config/example.project.yaml http_example
```

Pass secrets from the host with `-e`:

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -e API_TOKEN="$API_TOKEN" \
  -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  wiregoblin/wiregoblin:cli-latest \
  run -p /workspace/config/example.project.yaml http_example
```

This matches config entries like `api_token: ${API_TOKEN}` in the `secrets` section.

## Local examples

```bash
make compose-up
make run-http-example
make run-grpc-example
make run-local-stack-example
make run-goto-example
make run-error-handler-example
make run-workflow-block-example
make compose-down
```

Main example project: `config/example.project.yaml`.

# Go Envir

SSH deployment tool inspired by PHP Envoy

## Installation

```bash
go install github.com/yejune/go-envir@latest
```

Or build from source:

```bash
git clone https://github.com/yejune/go-envir.git
cd go-envir
go install .
```

## Quick Start

```bash
# 1. Initialize in your project folder
envir init

# 2. Edit Envirfile.yaml (server info, task settings)

# 3. Run deployment
envir deploy
```

## Commands

| Command | Description |
|---------|-------------|
| `envir` | Show available tasks (if Envirfile.yaml exists) |
| `envir init` | Create Envirfile.yaml template |
| `envir list` | List available tasks |
| `envir <task>` | Run a task |
| `envir <task> --on=<server>` | Run on specific server only |
| `envir help` | Show help |

## Envirfile.yaml Structure

```yaml
# Server definitions
servers:
  production:
    host: example.com
    user: ubuntu
    key: ~/.ssh/id_rsa    # default: ~/.ssh/id_rsa
    port: 22              # default: 22

  staging:
    host: staging.example.com
    user: deploy

# Task definitions
tasks:
  deploy:
    description: "Deploy to production"
    on: [production]      # servers to run on (defaults to first server)
    scripts:
      - local: echo "Building locally"
      - upload: ./app:/remote/path/app
      - run: sudo systemctl restart myapp

  logs:
    description: "View logs"
    on: [production]
    scripts:
      - run: sudo journalctl -u myapp -f
```

## Script Types

### local - Run command locally

```yaml
scripts:
  - local: GOOS=linux GOARCH=amd64 go build -o server .
  - local: npm run build
```

### upload - Upload file (SCP)

```yaml
scripts:
  - upload: ./server:/app/server-new
  - upload: ./dist:/var/www/html
```

### run - Run command on remote server

```yaml
scripts:
  - run: sudo systemctl restart myapp
  - run: |
      cd /app
      ./migrate.sh
      sudo systemctl restart myapp
```

## Example: Go Web Server Deployment

```yaml
servers:
  production:
    host: myserver.com
    user: ubuntu
    key: ~/.ssh/id_rsa

tasks:
  deploy:
    description: "Deploy to production"
    on: [production]
    scripts:
      # 1. Build for Linux locally
      - local: GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o server-linux .

      # 2. Upload to server
      - upload: server-linux:/app/server-new

      # 3. Replace and restart on server
      - run: |
          cd /app
          mv server server-old 2>/dev/null || true
          mv server-new server
          chmod +x server
          sudo systemctl restart myapp

      # 4. Clean up local build file
      - local: rm -f server-linux

  status:
    description: "Check service status"
    on: [production]
    scripts:
      - run: sudo systemctl status myapp --no-pager

  logs:
    description: "Real-time logs"
    on: [production]
    scripts:
      - run: sudo journalctl -u myapp -f

  rollback:
    description: "Rollback to previous version"
    on: [production]
    scripts:
      - run: |
          cd /app
          mv server server-failed
          mv server-old server
          sudo systemctl restart myapp
```

## Environment Variables

Environment variables can be used in Envirfile.yaml:

```yaml
servers:
  production:
    host: $DEPLOY_HOST
    user: $DEPLOY_USER
    key: $SSH_KEY_PATH
```

## Multi-Server Deployment

### Multiple Hosts (Array)

```yaml
servers:
  web:
    hosts:
      - web1.example.com
      - web2.example.com
      - web3.example.com
    user: ubuntu

tasks:
  deploy:
    on: [web]  # automatically expands to web[0], web[1], web[2]
    parallel: true
    scripts:
      - upload: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

### Sequential Execution (default)

```yaml
tasks:
  deploy:
    on: [web1, web2]  # runs sequentially
    scripts:
      - upload: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

### Parallel Execution

```yaml
tasks:
  deploy:
    on: [web1, web2, web3]
    parallel: true  # run on all servers simultaneously
    scripts:
      - upload: ./app:/app/server-new
      - run: sudo systemctl restart myapp
```

When running in parallel:
- Deploys to all servers simultaneously
- Output from each server is buffered and displayed in order
- Returns error if any server fails

### Specify Specific Server

```bash
envir deploy --on=web1
```

## License

MIT

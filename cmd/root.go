package cmd

import (
	"fmt"
	"os"

	"github.com/yejune/go-envir/internal/config"
	"github.com/yejune/go-envir/internal/runner"
)

func Execute(args []string) error {
	if len(args) < 1 {
		// 인자 없으면 task 목록 표시 (파일 없으면 help)
		if _, err := os.Stat("Envirfile.yaml"); os.IsNotExist(err) {
			printUsage()
			return nil
		}
		return listTasks()
	}

	command := args[0]

	switch command {
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: envir run <task> [--on=server]")
		}
		return runTask(args[1], parseServer(args))

	case "init":
		return initConfig()

	case "list":
		return listTasks()

	case "help", "--help", "-h":
		printUsage()
		return nil

	default:
		// 기본: task 이름으로 간주
		return runTask(command, parseServer(args))
	}
}

func runTask(taskName string, serverFilter string) error {
	cfg, err := config.Load("Envirfile.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	r := runner.New(cfg)
	defer r.Close()

	return r.Run(taskName, serverFilter)
}

func listTasks() error {
	cfg, err := config.Load("Envirfile.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Available tasks:")
	for name, task := range cfg.Tasks {
		desc := task.Description
		if desc == "" {
			desc = "(no description)"
		}
		fmt.Printf("  %-20s %s\n", name, desc)
	}
	return nil
}

func initConfig() error {
	if _, err := os.Stat("Envirfile.yaml"); err == nil {
		return fmt.Errorf("Envirfile.yaml already exists")
	}

	example := `# Envirfile.yaml - Go Envir 배포 설정
servers:
  production:
    host: example.com
    user: ubuntu
    key: ~/.ssh/id_rsa
    # port: 22

tasks:
  deploy:
    description: "Deploy to production"
    on: [production]
    scripts:
      - local: GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o server-linux .
      - upload: server-linux:/app/server-new
      - run: |
          cd /app
          mv server server-old 2>/dev/null || true
          mv server-new server
          chmod +x server
          sudo systemctl restart myapp
      - local: rm -f server-linux

  logs:
    description: "View logs"
    on: [production]
    scripts:
      - run: sudo journalctl -u myapp -f

  status:
    description: "Check service status"
    on: [production]
    scripts:
      - run: sudo systemctl status myapp --no-pager

  rollback:
    description: "Rollback to previous version"
    on: [production]
    scripts:
      - run: |
          cd /app
          mv server server-failed
          mv server-old server
          sudo systemctl restart myapp
`

	if err := os.WriteFile("Envirfile.yaml", []byte(example), 0644); err != nil {
		return fmt.Errorf("failed to write Envirfile.yaml: %w", err)
	}

	fmt.Println("Created Envirfile.yaml")
	return nil
}

func parseServer(args []string) string {
	for _, arg := range args {
		if len(arg) > 5 && arg[:5] == "--on=" {
			return arg[5:]
		}
	}
	return ""
}

func printUsage() {
	fmt.Println(`Go Envir - SSH deployment tool

Usage:
  envir <task>              Run a task
  envir run <task>          Run a task (explicit)
  envir run <task> --on=X   Run on specific server
  envir list                List available tasks
  envir init                Create example Envirfile.yaml

Examples:
  envir deploy              Deploy to production
  envir logs                View logs
  envir status              Check service status
  envir rollback            Rollback to previous version`)
}

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/yejune/gorelay/internal/config"
	"github.com/yejune/gorelay/internal/runner"
)

func Execute(args []string) error {
	if len(args) < 1 {
		// 인자 없으면 task 목록 표시 (파일 없으면 help)
		if _, err := os.Stat("Gorelayfile.yaml"); os.IsNotExist(err) {
			printUsage()
			return nil
		}
		return listTasks()
	}

	command := args[0]

	switch command {
	case "run":
		if len(args) < 2 {
			return fmt.Errorf("usage: gorelay run <task> [--on=server] [-v]")
		}
		return runTask(args[1], parseServer(args), parseVerbose(args))

	case "init":
		return initConfig()

	case "list":
		return listTasks()

	case "help", "--help", "-h":
		printUsage()
		return nil

	case "version", "--version", "-V":
		fmt.Printf("gorelay version %s\n", Version)
		return nil

	case "self-update", "selfupdate":
		return SelfUpdate()

	default:
		// 기본: task 이름으로 간주
		return runTask(command, parseServer(args), parseVerbose(args))
	}
}

func runTask(taskName string, serverFilter string, verbose bool) error {
	cfg, err := config.Load("Gorelayfile.yaml")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	r := runner.New(cfg)
	defer r.Close()

	if verbose {
		r.SetVerbose(true)
	}

	return r.Run(taskName, serverFilter)
}

func listTasks() error {
	cfg, err := config.Load("Gorelayfile.yaml")
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
	if _, err := os.Stat("Gorelayfile.yaml"); err == nil {
		return fmt.Errorf("Gorelayfile.yaml already exists")
	}

	example := `# Gorelayfile.yaml - Gorelay 배포 설정
servers:
  production:
    host: example.com
    user: ubuntu
    key: ~/.ssh/id_rsa
    # port: 22

# 로그 설정 (선택)
log:
  enabled: true
  path: ./gorelay.log

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

	if err := os.WriteFile("Gorelayfile.yaml", []byte(example), 0644); err != nil {
		return fmt.Errorf("failed to write Gorelayfile.yaml: %w", err)
	}

	fmt.Println("Created Gorelayfile.yaml")
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

func parseVerbose(args []string) bool {
	for _, arg := range args {
		if arg == "-v" || arg == "--verbose" || strings.HasPrefix(arg, "-v") {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Printf(`Gorelay - SSH deployment tool (version %s)

Usage:
  gorelay <task>              Run a task
  gorelay <task> -v           Run with verbose output
  gorelay run <task>          Run a task (explicit)
  gorelay run <task> --on=X   Run on specific server
  gorelay list                List available tasks
  gorelay init                Create example Gorelayfile.yaml
  gorelay version             Show version
  gorelay self-update         Update to latest version

Options:
  -v, --verbose             Show detailed output (timing, checksums, etc.)
  --on=<server>             Run on specific server only

Examples:
  gorelay deploy              Deploy to production
  gorelay deploy -v           Deploy with verbose output
  gorelay logs                View logs
  gorelay status              Check service status
  gorelay rollback            Rollback to previous version
`, Version)
}

package runner

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/yejune/go-envir/internal/config"
	"github.com/yejune/go-envir/internal/ssh"
)

type Runner struct {
	config  *config.EnvirConfig
	clients map[string]*ssh.Client
	stdout  io.Writer
	stderr  io.Writer
}

func New(cfg *config.EnvirConfig) *Runner {
	return &Runner{
		config:  cfg,
		clients: make(map[string]*ssh.Client),
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}
}

func (r *Runner) Close() {
	for _, client := range r.clients {
		client.Close()
	}
}

func (r *Runner) Run(taskName string, serverFilter string) error {
	task, ok := r.config.Tasks[taskName]
	if !ok {
		return fmt.Errorf("task '%s' not found", taskName)
	}

	// 서버 목록 결정
	servers := task.On
	if serverFilter != "" {
		servers = []string{serverFilter}
	}
	if len(servers) == 0 {
		// 기본 서버 사용
		for name := range r.config.Servers {
			servers = append(servers, name)
			break
		}
	}

	fmt.Printf("🚀 Running task: %s\n", taskName)

	for _, serverName := range servers {
		server, ok := r.config.Servers[serverName]
		if !ok {
			return fmt.Errorf("server '%s' not found", serverName)
		}

		fmt.Printf("\n📡 [%s] %s\n", serverName, server.Host)

		// 스크립트 실행
		for _, script := range task.Scripts {
			if err := r.runScript(serverName, server, script); err != nil {
				return fmt.Errorf("[%s] script failed: %w", serverName, err)
			}
		}
	}

	fmt.Printf("\n✅ Task '%s' completed\n", taskName)
	return nil
}

func (r *Runner) runScript(serverName string, server config.Server, script config.Script) error {
	// 로컬 실행
	if script.Local != "" {
		fmt.Printf("   ⚡ Local: %s\n", truncate(script.Local, 60))
		return r.runLocal(script.Local)
	}

	// 업로드
	if script.Upload != "" {
		parts := strings.SplitN(script.Upload, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid upload format: %s (expected 'local:remote')", script.Upload)
		}
		fmt.Printf("   📤 Upload: %s → %s\n", parts[0], parts[1])
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		return client.Upload(parts[0], parts[1])
	}

	// 원격 실행
	if script.Run != "" {
		fmt.Printf("   ▶ Run: %s\n", truncate(script.Run, 60))
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		return client.Run(script.Run, r.stdout, r.stderr)
	}

	return nil
}

func (r *Runner) runLocal(command string) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	return cmd.Run()
}

func (r *Runner) getClient(serverName string, server config.Server) (*ssh.Client, error) {
	if client, ok := r.clients[serverName]; ok {
		return client, nil
	}

	keyPath := server.Key
	if keyPath == "" {
		keyPath = "~/.ssh/id_rsa"
	}

	client, err := ssh.NewClient(server.Host, server.User, keyPath, server.Port)
	if err != nil {
		return nil, err
	}

	r.clients[serverName] = client
	return client, nil
}

func truncate(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}

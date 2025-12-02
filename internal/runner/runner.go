package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/yejune/go-envir/internal/config"
	"github.com/yejune/go-envir/internal/ssh"
)

type Runner struct {
	config  *config.EnvirConfig
	clients map[string]*ssh.Client
	mu      sync.Mutex
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
	var servers []string
	if serverFilter != "" {
		servers = []string{serverFilter}
	} else if len(task.On) > 0 {
		servers = task.On
	} else {
		// 기본 서버 사용
		for name := range r.config.Servers {
			servers = append(servers, name)
			break
		}
	}

	// 배열 host를 가진 서버는 자동 확장
	servers = r.config.GetExpandedServers(servers)

	fmt.Printf("🚀 Running task: %s", taskName)
	if task.Parallel && len(servers) > 1 {
		fmt.Printf(" (parallel)")
	}
	fmt.Println()

	// 병렬 실행
	if task.Parallel && len(servers) > 1 {
		return r.runParallel(task, servers)
	}

	// 순차 실행
	return r.runSequential(task, servers)
}

func (r *Runner) runSequential(task config.Task, servers []string) error {
	for _, serverName := range servers {
		server, ok := r.config.Servers[serverName]
		if !ok {
			return fmt.Errorf("server '%s' not found", serverName)
		}

		host := getHost(server)
		fmt.Printf("\n📡 [%s] %s\n", serverName, host)

		for _, script := range task.Scripts {
			if err := r.runScript(serverName, server, script, r.stdout, r.stderr); err != nil {
				return fmt.Errorf("[%s] script failed: %w", serverName, err)
			}
		}
	}

	fmt.Printf("\n✅ Task completed\n")
	return nil
}

func (r *Runner) runParallel(task config.Task, servers []string) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(servers))
	results := make(map[string]*bytes.Buffer)
	var resultsMu sync.Mutex

	for _, serverName := range servers {
		server, ok := r.config.Servers[serverName]
		if !ok {
			return fmt.Errorf("server '%s' not found", serverName)
		}

		wg.Add(1)
		go func(srvName string, srv config.Server) {
			defer wg.Done()

			// 각 서버별 출력 버퍼
			buf := &bytes.Buffer{}
			buf.WriteString(fmt.Sprintf("\n📡 [%s] %s\n", srvName, getHost(srv)))

			for _, script := range task.Scripts {
				if err := r.runScript(srvName, srv, script, buf, buf); err != nil {
					buf.WriteString(fmt.Sprintf("   ❌ Error: %v\n", err))
					errCh <- fmt.Errorf("[%s] %w", srvName, err)
					resultsMu.Lock()
					results[srvName] = buf
					resultsMu.Unlock()
					return
				}
			}

			buf.WriteString(fmt.Sprintf("   ✓ Done\n"))
			resultsMu.Lock()
			results[srvName] = buf
			resultsMu.Unlock()
		}(serverName, server)
	}

	wg.Wait()
	close(errCh)

	// 결과 출력 (순서대로)
	for _, serverName := range servers {
		if buf, ok := results[serverName]; ok {
			fmt.Print(buf.String())
		}
	}

	// 에러 수집
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		fmt.Printf("\n❌ %d server(s) failed\n", len(errs))
		return errs[0]
	}

	fmt.Printf("\n✅ All %d servers completed\n", len(servers))
	return nil
}

func (r *Runner) runScript(serverName string, server config.Server, script config.Script, stdout, stderr io.Writer) error {
	// 로컬 실행
	if script.Local != "" {
		fmt.Fprintf(stdout, "   ⚡ Local: %s\n", truncate(script.Local, 60))
		return r.runLocal(script.Local, stdout, stderr)
	}

	// 업로드
	if script.Upload != "" {
		parts := strings.SplitN(script.Upload, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid upload format: %s (expected 'local:remote')", script.Upload)
		}
		fmt.Fprintf(stdout, "   📤 Upload: %s → %s\n", parts[0], parts[1])
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		return client.Upload(parts[0], parts[1])
	}

	// 원격 실행
	if script.Run != "" {
		fmt.Fprintf(stdout, "   ▶ Run: %s\n", truncate(script.Run, 60))
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		return client.Run(script.Run, stdout, stderr)
	}

	return nil
}

func (r *Runner) runLocal(command string, stdout, stderr io.Writer) error {
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func (r *Runner) getClient(serverName string, server config.Server) (*ssh.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.clients[serverName]; ok {
		return client, nil
	}

	keyPath := server.Key
	if keyPath == "" {
		keyPath = "~/.ssh/id_rsa"
	}

	client, err := ssh.NewClient(getHost(server), server.User, keyPath, server.Port)
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

// getHost extracts host string from Server
func getHost(server config.Server) string {
	if len(server.Hosts) > 0 {
		return server.Hosts[0]
	}
	return server.Host
}

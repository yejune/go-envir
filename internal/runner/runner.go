package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yejune/go-envir/internal/config"
	"github.com/yejune/go-envir/internal/ssh"
)

type Runner struct {
	config  *config.EnvirConfig
	clients map[string]*ssh.Client
	mu      sync.Mutex
	stdout  io.Writer
	stderr  io.Writer
	verbose bool
	logFile *os.File
}

func New(cfg *config.EnvirConfig) *Runner {
	r := &Runner{
		config:  cfg,
		clients: make(map[string]*ssh.Client),
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}

	// 로그 파일 설정
	if cfg.Log.Enabled {
		logPath := cfg.Log.Path
		if logPath == "" {
			logPath = "envir.log"
		}
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			r.logFile = f
		}
	}

	return r
}

func (r *Runner) SetVerbose(v bool) {
	r.verbose = v
	// verbose 모드면 SSH 클라이언트에도 전달
	for _, client := range r.clients {
		client.SetVerbose(v)
	}
}

func (r *Runner) Close() {
	for _, client := range r.clients {
		client.Close()
	}
	if r.logFile != nil {
		r.logFile.Close()
	}
}

func (r *Runner) log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)

	// 콘솔 출력
	fmt.Print(msg)

	// 파일 로그
	if r.logFile != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		// 이모지 제거하고 로그
		cleanMsg := strings.TrimSpace(msg)
		r.logFile.WriteString(fmt.Sprintf("[%s] %s\n", timestamp, cleanMsg))
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

	r.log("🚀 Running task: %s", taskName)
	if task.Parallel && len(servers) > 1 {
		r.log(" (parallel)")
	}
	r.log("\n")

	startTime := time.Now()

	var err error
	// 병렬 실행
	if task.Parallel && len(servers) > 1 {
		err = r.runParallel(task, servers)
	} else {
		// 순차 실행
		err = r.runSequential(task, servers)
	}

	elapsed := time.Since(startTime)
	if r.verbose {
		r.log("   ⏱ Elapsed: %s\n", elapsed.Round(time.Millisecond))
	}

	return err
}

func (r *Runner) runSequential(task config.Task, servers []string) error {
	for _, serverName := range servers {
		server, ok := r.config.Servers[serverName]
		if !ok {
			return fmt.Errorf("server '%s' not found", serverName)
		}

		host := getHost(server)
		r.log("\n📡 [%s] %s\n", serverName, host)

		for _, script := range task.Scripts {
			if err := r.runScript(serverName, server, script, r.stdout, r.stderr); err != nil {
				r.log("   ❌ Error: %v\n", err)
				return fmt.Errorf("[%s] script failed: %w", serverName, err)
			}
		}
	}

	r.log("\n✅ Task completed\n")
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
			r.log("%s", buf.String())
		}
	}

	// 에러 수집
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		r.log("\n❌ %d server(s) failed\n", len(errs))
		return errs[0]
	}

	r.log("\n✅ All %d servers completed\n", len(servers))
	return nil
}

func (r *Runner) runScript(serverName string, server config.Server, script config.Script, stdout, stderr io.Writer) error {
	startTime := time.Now()

	// 로컬 실행
	if script.Local != "" {
		r.logScript(stdout, "⚡ Local", script.Local)
		err := r.runLocal(script.Local, stdout, stderr)
		r.logElapsed(stdout, startTime)
		return err
	}

	// 파일 업로드
	if script.Upload != "" {
		parts := strings.SplitN(script.Upload, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid upload format: %s (expected 'local:remote')", script.Upload)
		}
		r.logScript(stdout, "📤 Upload", fmt.Sprintf("%s → %s", parts[0], parts[1]))
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		err = client.Upload(parts[0], parts[1])
		r.logElapsed(stdout, startTime)
		return err
	}

	// 디렉토리 업로드 (변경분만, rsync 스타일)
	if script.UploadDir != "" {
		parts := strings.SplitN(script.UploadDir, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid upload_dir format: %s (expected 'local:remote')", script.UploadDir)
		}
		r.logScript(stdout, "📁 Sync", fmt.Sprintf("%s → %s", parts[0], parts[1]))
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		uploaded, err := client.UploadDir(parts[0], parts[1])
		if err == nil {
			fmt.Fprintf(stdout, "      %d file(s) uploaded\n", uploaded)
		}
		r.logElapsed(stdout, startTime)
		return err
	}

	// 디렉토리 tar 업로드 (압축해서 전체 교체)
	if script.UploadTar != "" {
		parts := strings.SplitN(script.UploadTar, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid upload_tar format: %s (expected 'local:remote')", script.UploadTar)
		}
		r.logScript(stdout, "📦 Tar", fmt.Sprintf("%s → %s", parts[0], parts[1]))
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		err = client.UploadDirTar(parts[0], parts[1])
		r.logElapsed(stdout, startTime)
		return err
	}

	// 원격 실행
	if script.Run != "" {
		r.logScript(stdout, "▶ Run", script.Run)
		client, err := r.getClient(serverName, server)
		if err != nil {
			return err
		}
		err = client.Run(script.Run, stdout, stderr)
		r.logElapsed(stdout, startTime)
		return err
	}

	return nil
}

func (r *Runner) logScript(w io.Writer, prefix, cmd string) {
	msg := fmt.Sprintf("   %s: %s\n", prefix, truncate(cmd, 60))
	fmt.Fprint(w, msg)
	if r.logFile != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		r.logFile.WriteString(fmt.Sprintf("[%s] %s: %s\n", timestamp, prefix, cmd))
	}
}

func (r *Runner) logElapsed(w io.Writer, startTime time.Time) {
	if r.verbose {
		elapsed := time.Since(startTime)
		fmt.Fprintf(w, "      ⏱ %s\n", elapsed.Round(time.Millisecond))
	}
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

	client.SetVerbose(r.verbose)
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

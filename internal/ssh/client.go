package ssh

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Client struct {
	conn    *ssh.Client
	host    string
	config  *ssh.ClientConfig
	verbose bool
}

func NewClient(host, user, keyPath string, port int) (*Client, error) {
	if port == 0 {
		port = 22
	}

	// SSH 키 읽기
	key, err := os.ReadFile(expandPath(keyPath))
	if err != nil {
		return nil, fmt.Errorf("failed to read SSH key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to parse SSH key: %w", err)
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}

	return &Client{
		conn:   conn,
		host:   host,
		config: config,
	}, nil
}

func (c *Client) SetVerbose(v bool) {
	c.verbose = v
}

func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) Run(command string, stdout, stderr io.Writer) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	session.Stdout = stdout
	session.Stderr = stderr

	return session.Run(command)
}

func (c *Client) Upload(localPath, remotePath string) error {
	// 1. 로컬 파일 읽기 및 체크섬 계산
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}

	// 파일 내용 읽어서 체크섬 계산
	fileContent, err := io.ReadAll(localFile)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	localHash := sha256.Sum256(fileContent)
	localHashStr := hex.EncodeToString(localHash[:])

	if c.verbose {
		fmt.Printf("      Local file: %s (%d bytes)\n", localPath, stat.Size())
		fmt.Printf("      Local SHA256: %s\n", localHashStr)
	}

	// 2. SCP로 파일 전송
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// stdin/stdout/stderr 파이프
	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	var scpStdout, scpStderr bytes.Buffer
	session.Stdout = &scpStdout
	session.Stderr = &scpStderr

	// SCP 명령 시작
	if err := session.Start(fmt.Sprintf("/usr/bin/scp -t %s", remotePath)); err != nil {
		return fmt.Errorf("failed to start scp: %w", err)
	}

	// SCP 프로토콜: 파일 헤더 전송
	header := fmt.Sprintf("C0644 %d %s\n", stat.Size(), filepath.Base(remotePath))
	if _, err := stdinPipe.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write scp header: %w", err)
	}

	// 파일 내용 전송
	if _, err := stdinPipe.Write(fileContent); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// 종료 바이트
	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to write terminator: %w", err)
	}

	stdinPipe.Close()

	// SCP 완료 대기
	if err := session.Wait(); err != nil {
		return fmt.Errorf("scp failed: %w (stderr: %s)", err, scpStderr.String())
	}

	if c.verbose {
		fmt.Printf("      SCP transfer completed\n")
	}

	// 3. 원격 파일 체크섬 검증
	remoteHashStr, err := c.getRemoteChecksum(remotePath)
	if err != nil {
		return fmt.Errorf("failed to verify remote checksum: %w", err)
	}

	if c.verbose {
		fmt.Printf("      Remote SHA256: %s\n", remoteHashStr)
	}

	if localHashStr != remoteHashStr {
		return fmt.Errorf("checksum mismatch: local=%s, remote=%s", localHashStr, remoteHashStr)
	}

	if c.verbose {
		fmt.Printf("      ✓ Checksum verified\n")
	}

	return nil
}

func (c *Client) getRemoteChecksum(remotePath string) (string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// sha256sum 또는 shasum 시도
	cmd := fmt.Sprintf("sha256sum %s 2>/dev/null || shasum -a 256 %s 2>/dev/null", remotePath, remotePath)
	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("checksum command failed: %w (stderr: %s)", err, stderr.String())
	}

	// 출력에서 해시 추출 (첫 번째 필드)
	output := strings.TrimSpace(stdout.String())
	parts := strings.Fields(output)
	if len(parts) < 1 {
		return "", fmt.Errorf("unexpected checksum output: %s", output)
	}

	return parts[0], nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

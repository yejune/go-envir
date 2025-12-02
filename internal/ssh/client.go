package ssh

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// Upload uploads a single file with checksum verification
func (c *Client) Upload(localPath, remotePath string) error {
	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local path: %w", err)
	}

	if stat.IsDir() {
		return fmt.Errorf("use UploadDir for directories: %s", localPath)
	}

	return c.uploadFile(localPath, remotePath)
}

// UploadDir uploads a directory (changed files only, rsync-style)
func (c *Client) UploadDir(localDir, remoteDir string) (int, error) {
	// 원격 디렉토리 생성
	if err := c.Run(fmt.Sprintf("mkdir -p %s", remoteDir), io.Discard, io.Discard); err != nil {
		return 0, fmt.Errorf("failed to create remote directory: %w", err)
	}

	// 원격 파일 체크섬 목록 가져오기
	remoteChecksums, err := c.getRemoteDirChecksums(remoteDir)
	if err != nil {
		if c.verbose {
			fmt.Printf("      No existing files on remote (new directory)\n")
		}
		remoteChecksums = make(map[string]string)
	}

	uploaded := 0
	skipped := 0

	err = filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 상대 경로 계산
		relPath, _ := filepath.Rel(localDir, path)
		remotePath := filepath.Join(remoteDir, relPath)

		if info.IsDir() {
			// 원격 디렉토리 생성
			return c.Run(fmt.Sprintf("mkdir -p %s", remotePath), io.Discard, io.Discard)
		}

		// 로컬 파일 체크섬 계산
		localChecksum, err := c.getLocalChecksum(path)
		if err != nil {
			return err
		}

		// 원격 체크섬과 비교
		if remoteChecksum, ok := remoteChecksums[relPath]; ok && remoteChecksum == localChecksum {
			skipped++
			if c.verbose {
				fmt.Printf("      Skip (unchanged): %s\n", relPath)
			}
			return nil
		}

		// 변경된 파일만 업로드
		if c.verbose {
			fmt.Printf("      Upload: %s\n", relPath)
		}
		if err := c.uploadFile(path, remotePath); err != nil {
			return err
		}
		uploaded++
		return nil
	})

	if c.verbose {
		fmt.Printf("      Uploaded: %d, Skipped: %d\n", uploaded, skipped)
	}

	return uploaded, err
}

// UploadDirTar uploads a directory as tar.gz and extracts on remote
func (c *Client) UploadDirTar(localDir, remoteDir string) error {
	// 1. 로컬에서 tar.gz 생성 (메모리)
	var tarBuffer bytes.Buffer
	gzWriter := gzip.NewWriter(&tarBuffer)
	tarWriter := tar.NewWriter(gzWriter)

	err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(localDir, path)
		if relPath == "." {
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create tar: %w", err)
	}

	tarWriter.Close()
	gzWriter.Close()

	tarContent := tarBuffer.Bytes()
	tarHash := sha256.Sum256(tarContent)
	tarHashStr := hex.EncodeToString(tarHash[:])

	if c.verbose {
		fmt.Printf("      Tar size: %d bytes\n", len(tarContent))
		fmt.Printf("      Tar SHA256: %s\n", tarHashStr)
	}

	// 2. 원격에 임시 파일로 업로드
	remoteTar := fmt.Sprintf("/tmp/envir-%s.tar.gz", tarHashStr[:8])
	if err := c.uploadBytes(tarContent, remoteTar); err != nil {
		return fmt.Errorf("failed to upload tar: %w", err)
	}

	// 3. 원격에서 압축 해제
	extractCmd := fmt.Sprintf("mkdir -p %s && tar -xzf %s -C %s && rm -f %s", remoteDir, remoteTar, remoteDir, remoteTar)
	var stderr bytes.Buffer
	if err := c.Run(extractCmd, io.Discard, &stderr); err != nil {
		return fmt.Errorf("failed to extract tar: %w (stderr: %s)", err, stderr.String())
	}

	if c.verbose {
		fmt.Printf("      ✓ Extracted to %s\n", remoteDir)
	}

	return nil
}

func (c *Client) uploadFile(localPath, remotePath string) error {
	// 1. 로컬 파일 읽기 및 체크섬 계산
	fileContent, err := os.ReadFile(localPath)
	if err != nil {
		return fmt.Errorf("failed to read local file: %w", err)
	}

	stat, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
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

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	var scpStderr bytes.Buffer
	session.Stderr = &scpStderr

	// 원격 디렉토리 생성
	remoteDir := filepath.Dir(remotePath)
	c.Run(fmt.Sprintf("mkdir -p %s", remoteDir), io.Discard, io.Discard)

	// SCP 명령 시작 (-t: sink mode)
	if err := session.Start(fmt.Sprintf("/usr/bin/scp -t %s", remotePath)); err != nil {
		return fmt.Errorf("failed to start scp: %w", err)
	}

	// SCP 프로토콜: 파일 헤더 전송 (C<mode> <size> <filename>)
	header := fmt.Sprintf("C0644 %d %s\n", stat.Size(), filepath.Base(remotePath))
	if _, err := stdinPipe.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write scp header: %w", err)
	}

	// 파일 내용 전송
	if _, err := stdinPipe.Write(fileContent); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// 종료 바이트 (0x00)
	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to write terminator: %w", err)
	}

	stdinPipe.Close()

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

func (c *Client) uploadBytes(content []byte, remotePath string) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	var scpStderr bytes.Buffer
	session.Stderr = &scpStderr

	if err := session.Start(fmt.Sprintf("/usr/bin/scp -t %s", remotePath)); err != nil {
		return fmt.Errorf("failed to start scp: %w", err)
	}

	header := fmt.Sprintf("C0644 %d %s\n", len(content), filepath.Base(remotePath))
	if _, err := stdinPipe.Write([]byte(header)); err != nil {
		return fmt.Errorf("failed to write scp header: %w", err)
	}

	if _, err := stdinPipe.Write(content); err != nil {
		return fmt.Errorf("failed to write content: %w", err)
	}

	if _, err := stdinPipe.Write([]byte{0}); err != nil {
		return fmt.Errorf("failed to write terminator: %w", err)
	}

	stdinPipe.Close()

	if err := session.Wait(); err != nil {
		return fmt.Errorf("scp failed: %w (stderr: %s)", err, scpStderr.String())
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

	cmd := fmt.Sprintf("sha256sum %s 2>/dev/null || shasum -a 256 %s 2>/dev/null", remotePath, remotePath)
	if err := session.Run(cmd); err != nil {
		return "", fmt.Errorf("checksum command failed: %w (stderr: %s)", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	parts := strings.Fields(output)
	if len(parts) < 1 {
		return "", fmt.Errorf("unexpected checksum output: %s", output)
	}

	return parts[0], nil
}

func (c *Client) getRemoteDirChecksums(remoteDir string) (map[string]string, error) {
	session, err := c.conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	// 모든 파일의 체크섬 가져오기
	cmd := fmt.Sprintf("find %s -type f -exec sha256sum {} \\; 2>/dev/null || find %s -type f -exec shasum -a 256 {} \\; 2>/dev/null", remoteDir, remoteDir)
	if err := session.Run(cmd); err != nil {
		return nil, err
	}

	checksums := make(map[string]string)
	for _, line := range strings.Split(stdout.String(), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			hash := parts[0]
			path := parts[1]
			// 상대 경로로 변환
			relPath := strings.TrimPrefix(path, remoteDir+"/")
			checksums[relPath] = hash
		}
	}

	return checksums, nil
}

func (c *Client) getLocalChecksum(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

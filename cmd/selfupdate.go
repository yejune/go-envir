package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	githubAPI   = "https://api.github.com/repos/yejune/go-envir/releases/latest"
	downloadURL = "https://github.com/yejune/go-envir/releases/download/%s/envir-%s-%s"
)

func SelfUpdate() error {
	fmt.Println("🔍 Checking for updates...")

	// Get latest version from GitHub
	latestVersion, err := getLatestVersion()
	if err != nil {
		return fmt.Errorf("failed to check latest version: %w", err)
	}

	// Get current executable path
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	exePath, err = filepath.EvalSymlinks(exePath)
	if err != nil {
		return fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	fmt.Printf("Current version: %s\n", Version)
	fmt.Printf("Latest version:  %s\n", latestVersion)
	fmt.Printf("Current path:    %s\n\n", exePath)

	// Compare versions
	if Version == latestVersion {
		fmt.Println("✅ Already up to date!")
		return nil
	}

	// Determine platform
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	// Build download URL
	downloadPath := fmt.Sprintf(downloadURL, latestVersion, goos, goarch)

	fmt.Printf("📦 Downloading %s...\n", latestVersion)

	// Download binary
	tmpFile := filepath.Join(os.TempDir(), "envir-new")
	if err := downloadFile(tmpFile, downloadPath); err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer os.Remove(tmpFile)

	// Make executable
	if err := os.Chmod(tmpFile, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Println("✓ Download complete")
	fmt.Println()

	// Replace binary
	fmt.Println("📋 Installing update...")

	// If installed in /usr/local/bin, need sudo
	needsSudo := strings.HasPrefix(exePath, "/usr/local")

	var replaceCmd *exec.Cmd
	if needsSudo {
		fmt.Println("(sudo required)")
		replaceCmd = exec.Command("sudo", "mv", tmpFile, exePath)
	} else {
		replaceCmd = exec.Command("mv", tmpFile, exePath)
	}

	replaceCmd.Stdin = os.Stdin
	replaceCmd.Stdout = os.Stdout
	replaceCmd.Stderr = os.Stderr

	if err := replaceCmd.Run(); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Ensure executable
	if needsSudo {
		chmodCmd := exec.Command("sudo", "chmod", "+x", exePath)
		chmodCmd.Run()
	}

	fmt.Println()
	fmt.Println("✅ Update complete!")
	fmt.Printf("envir has been updated to %s\n", latestVersion)

	return nil
}

func getLatestVersion() (string, error) {
	resp, err := http.Get(githubAPI)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	// Simple parsing - just get tag_name from JSON
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	bodyStr := string(body)

	// Find "tag_name":"v1.2.3"
	tagStart := strings.Index(bodyStr, `"tag_name":"`)
	if tagStart == -1 {
		return "", fmt.Errorf("could not find tag_name in response")
	}

	tagStart += len(`"tag_name":"`)
	tagEnd := strings.Index(bodyStr[tagStart:], `"`)
	if tagEnd == -1 {
		return "", fmt.Errorf("could not parse tag_name")
	}

	return bodyStr[tagStart : tagStart+tagEnd], nil
}

func downloadFile(filepath string, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

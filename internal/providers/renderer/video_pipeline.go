package renderer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
)

// videoMkdirTemp, videoGetwd, and videoStatfs are indirection points for
// testing OS error paths.
var videoMkdirTemp = os.MkdirTemp
var videoGetwd = os.Getwd
var videoStatfs = syscall.Statfs

// CommandRunner is the signature for executing an external command.
// The default uses exec.CommandContext; tests inject a mock.
type CommandRunner func(ctx context.Context, name string, args ...string) error

type VideoPipeline struct {
	enabled    bool
	RunnerFn   CommandRunner
	LookPathFn func(file string) (string, error)
}

func NewVideoPipeline(enabled bool) *VideoPipeline {
	return &VideoPipeline{
		enabled: enabled,
		RunnerFn: func(ctx context.Context, name string, args ...string) error {
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Stdout = nil
			cmd.Stderr = nil
			return cmd.Run()
		},
		LookPathFn: exec.LookPath,
	}
}

func (p *VideoPipeline) IsEnabled() bool { return p.enabled }

// Format returns the renderer format identifier, satisfying the FormatRenderer
// interface so VideoPipeline can be registered alongside Markdown / HTML / PDF
// in the content generator's renderer slice.
func (p *VideoPipeline) Format() string { return "video" }

// SupportedContentTypes reports the MIME types this renderer produces. The
// current ffmpeg pipeline emits H.264 MP4.
func (p *VideoPipeline) SupportedContentTypes() []string {
	return []string{"video/mp4"}
}

func (p *VideoPipeline) Render(ctx context.Context, content models.Content, opts RenderOptions) ([]byte, error) {
	if !p.enabled {
		return nil, fmt.Errorf("video generation is not enabled")
	}

	// Check if ffmpeg is available
	if _, err := p.LookPathFn("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found: video generation requires ffmpeg installed")
	}

	// Create temporary directory for video generation
	tmpDir, err := videoMkdirTemp("", "patreon-video-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outputPath := filepath.Join(tmpDir, "output.mp4")

	// Generate a simple video with ffmpeg: blue background with title text
	duration := 5 // seconds
	args := []string{
		"-f", "lavfi",
		"-i", fmt.Sprintf("color=c=blue:s=1280x720:d=%d", duration),
		"-vf", fmt.Sprintf("drawtext=text='%s':fontcolor=white:fontsize=24:x=(w-text_w)/2:y=(h-text_h)/2", escapeFFmpegText(content.Title)),
		"-c:v", "libx264",
		"-t", fmt.Sprintf("%d", duration),
		"-y",
		outputPath,
	}

	if err := p.RunnerFn(ctx, "ffmpeg", args...); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w", err)
	}

	// Read generated video
	videoData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("read video file: %w", err)
	}

	return videoData, nil
}

func (p *VideoPipeline) CheckDiskSpace(requiredMB int) error {
	var stat syscall.Statfs_t
	wd, err := videoGetwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}
	if err := videoStatfs(wd, &stat); err != nil {
		return fmt.Errorf("statfs: %w", err)
	}

	// Calculate available space in MB
	availableBytes := stat.Bavail * uint64(stat.Bsize)
	requiredBytes := uint64(requiredMB) * 1024 * 1024

	if availableBytes < requiredBytes {
		return fmt.Errorf("insufficient disk space: %d MB required, %d MB available", requiredMB, availableBytes/(1024*1024))
	}
	return nil
}

func escapeFFmpegText(text string) string {
	// Simple escaping for ffmpeg drawtext: escape single quotes and backslashes
	// Replace ' with '\''
	// Replace \ with \\
	// This is basic; for production use a proper escaping function
	result := ""
	for _, r := range text {
		if r == '\'' {
			result += "'\\''"
		} else if r == '\\' {
			result += "\\\\"
		} else {
			result += string(r)
		}
	}
	return result
}

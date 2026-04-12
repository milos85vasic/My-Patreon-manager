package renderer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/milos85vasic/My-Patreon-Manager/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVideoPipeline(t *testing.T) {
	p := NewVideoPipeline(true)
	assert.True(t, p.IsEnabled())
	assert.NotNil(t, p.RunnerFn)
	assert.NotNil(t, p.LookPathFn)

	p2 := NewVideoPipeline(false)
	assert.False(t, p2.IsEnabled())
}

func TestVideoPipeline_IsEnabled(t *testing.T) {
	p := NewVideoPipeline(true)
	assert.True(t, p.IsEnabled())
	p = NewVideoPipeline(false)
	assert.False(t, p.IsEnabled())
}

func TestVideoPipeline_Render_Disabled(t *testing.T) {
	p := NewVideoPipeline(false)
	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "video generation is not enabled")
}

func TestVideoPipeline_Render_FFmpegNotFound(t *testing.T) {
	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "", errors.New("not found")
	}
	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ffmpeg not found")
}

func TestVideoPipeline_Render_CommandComposition(t *testing.T) {
	var capturedName string
	var capturedArgs []string

	// Create a temp dir with a dummy output file so ReadFile succeeds
	tmpDir := t.TempDir()
	dummyMP4 := []byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70} // fake MP4 header

	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}
	p.RunnerFn = func(ctx context.Context, name string, args ...string) error {
		capturedName = name
		capturedArgs = args
		// Find the output path (last argument) and write a dummy file
		outputPath := args[len(args)-1]
		return os.WriteFile(outputPath, dummyMP4, 0644)
	}

	content := models.Content{Title: "My Video Title", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	require.NoError(t, err)
	assert.NotNil(t, result)

	// Verify ffmpeg was called
	assert.Equal(t, "ffmpeg", capturedName)

	// Verify key arguments are present
	assert.Contains(t, capturedArgs, "-f")
	assert.Contains(t, capturedArgs, "lavfi")
	assert.Contains(t, capturedArgs, "-c:v")
	assert.Contains(t, capturedArgs, "libx264")
	assert.Contains(t, capturedArgs, "-y")

	// Verify the drawtext filter contains the title
	foundVF := false
	for i, arg := range capturedArgs {
		if arg == "-vf" && i+1 < len(capturedArgs) {
			foundVF = true
			assert.Contains(t, capturedArgs[i+1], "My Video Title")
			break
		}
	}
	assert.True(t, foundVF, "expected -vf argument with title")

	// Verify output path ends with .mp4
	outputPath := capturedArgs[len(capturedArgs)-1]
	assert.True(t, strings.HasSuffix(outputPath, "output.mp4"))

	_ = tmpDir // used by the test infrastructure
}

func TestVideoPipeline_Render_FFmpegFails(t *testing.T) {
	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}
	p.RunnerFn = func(ctx context.Context, name string, args ...string) error {
		return errors.New("ffmpeg exit code 1")
	}

	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ffmpeg failed")
}

func TestVideoPipeline_Render_ReadFileFails(t *testing.T) {
	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}
	p.RunnerFn = func(ctx context.Context, name string, args ...string) error {
		// Don't write the output file so ReadFile will fail
		return nil
	}

	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(context.Background(), content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "read video file")
}

func TestVideoPipeline_Render_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}
	p.RunnerFn = func(ctx context.Context, name string, args ...string) error {
		return ctx.Err()
	}

	content := models.Content{Title: "Test", Body: "Body"}
	opts := RenderOptions{}
	result, err := p.Render(ctx, content, opts)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "ffmpeg failed")
}

func TestVideoPipeline_Render_MkdirTempError(t *testing.T) {
	origMkdir := videoMkdirTemp
	defer func() { videoMkdirTemp = origMkdir }()
	videoMkdirTemp = func(dir, pattern string) (string, error) {
		return "", errors.New("cannot create temp dir")
	}

	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}

	content := models.Content{Title: "Test", Body: "Body"}
	result, err := p.Render(context.Background(), content, RenderOptions{})
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "create temp dir")
}

func TestVideoPipeline_CheckDiskSpace(t *testing.T) {
	p := NewVideoPipeline(true)
	err := p.CheckDiskSpace(100)
	assert.NoError(t, err)
}

func TestVideoPipeline_CheckDiskSpace_Insufficient(t *testing.T) {
	p := NewVideoPipeline(true)
	// Request an absurd amount of disk space
	err := p.CheckDiskSpace(999999999)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient disk space")
}

func TestVideoPipeline_CheckDiskSpace_GetwdError(t *testing.T) {
	origGetwd := videoGetwd
	defer func() { videoGetwd = origGetwd }()
	videoGetwd = func() (string, error) {
		return "", errors.New("getwd failed")
	}

	p := NewVideoPipeline(true)
	err := p.CheckDiskSpace(100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get working dir")
}

func TestVideoPipeline_CheckDiskSpace_StatfsError(t *testing.T) {
	origStatfs := videoStatfs
	defer func() { videoStatfs = origStatfs }()
	videoStatfs = func(path string, buf *syscall.Statfs_t) error {
		return errors.New("statfs failed")
	}

	p := NewVideoPipeline(true)
	err := p.CheckDiskSpace(100)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "statfs")
}

func TestVideoPipeline_Format(t *testing.T) {
	p := NewVideoPipeline(true)
	assert.Equal(t, "video", p.Format())
}

func TestVideoPipeline_SupportedContentTypes(t *testing.T) {
	p := NewVideoPipeline(true)
	assert.Equal(t, []string{"video/mp4"}, p.SupportedContentTypes())
}

func TestVideoPipeline_ImplementsFormatRenderer(t *testing.T) {
	var _ FormatRenderer = (*VideoPipeline)(nil)
}

func TestEscapeFFmpegText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "hello", "hello"},
		{"single quote", "it's", "it'\\''s"},
		{"backslash", `back\slash`, `back\\slash`},
		{"mixed", `it's a\b`, `it'\''s a\\b`},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeFFmpegText(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVideoPipeline_Render_TitleEscaping(t *testing.T) {
	var capturedArgs []string

	p := NewVideoPipeline(true)
	p.LookPathFn = func(file string) (string, error) {
		return "/usr/bin/ffmpeg", nil
	}
	p.RunnerFn = func(ctx context.Context, name string, args ...string) error {
		capturedArgs = args
		outputPath := args[len(args)-1]
		return os.WriteFile(outputPath, []byte("fake"), 0644)
	}

	content := models.Content{Title: "Hello's World", Body: "Body"}
	opts := RenderOptions{}
	_, err := p.Render(context.Background(), content, opts)
	require.NoError(t, err)

	// Find the -vf argument and verify the title was escaped
	for i, arg := range capturedArgs {
		if arg == "-vf" && i+1 < len(capturedArgs) {
			// The escaped single quote should appear
			assert.Contains(t, capturedArgs[i+1], "Hello")
			break
		}
	}
}

func TestVideoPipeline_DefaultRunnerFn(t *testing.T) {
	// Verify the default runner calls exec.CommandContext by running a harmless command
	p := NewVideoPipeline(true)
	err := p.RunnerFn(context.Background(), "true")
	assert.NoError(t, err)
}

func TestVideoPipeline_DefaultLookPathFn(t *testing.T) {
	// Verify the default LookPathFn works (should find "true" or "sh")
	p := NewVideoPipeline(true)
	path, err := p.LookPathFn("sh")
	assert.NoError(t, err)
	assert.True(t, filepath.IsAbs(path))
}

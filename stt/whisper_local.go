// Package stt provides speech-to-text provider interface and implementations.
package stt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// WhisperLocal implements the Provider interface using local whisper.cpp.
// It uses the whisper-cpp CLI tool for transcription.
type WhisperLocal struct {
	modelPath string
	modelSize string // "tiny", "base", "small", "medium", "large"
	binPath   string // Path to whisper-cpp binary

	mu            sync.RWMutex
	ready         bool
	hasBinary     bool
	setupProgress int
}

// WhisperLocalConfig holds configuration for WhisperLocal.
type WhisperLocalConfig struct {
	ModelSize string // "tiny", "base", "small", "medium", "large"
	ModelDir  string // Directory to store models
	BinPath   string // Path to whisper-cpp binary (optional, will download if not set)
}

// Model sizes and their approximate download sizes.
var modelSizes = map[string]struct {
	URL        string
	Size       int64 // Approximate size in bytes
	SpeedRatio float64
}{
	"tiny":   {"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.bin", 75 * 1024 * 1024, 10.0},
	"base":   {"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin", 150 * 1024 * 1024, 7.0},
	"small":  {"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin", 500 * 1024 * 1024, 4.0},
	"medium": {"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-medium.bin", 1500 * 1024 * 1024, 2.0},
	"large":  {"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin", 3000 * 1024 * 1024, 1.0},
}

// NewWhisperLocal creates a new WhisperLocal provider.
func NewWhisperLocal(cfg WhisperLocalConfig) (*WhisperLocal, error) {
	if cfg.ModelSize == "" {
		cfg.ModelSize = "base"
	}

	if _, ok := modelSizes[cfg.ModelSize]; !ok {
		return nil, fmt.Errorf("invalid model size: %s", cfg.ModelSize)
	}

	if cfg.ModelDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cfg.ModelDir = filepath.Join(homeDir, ".transy", "models")
	}

	w := &WhisperLocal{
		modelSize:     cfg.ModelSize,
		modelPath:     filepath.Join(cfg.ModelDir, fmt.Sprintf("ggml-%s.bin", cfg.ModelSize)),
		binPath:       cfg.BinPath,
		setupProgress: -1,
	}

	// Check if binary exists
	if binPath := w.findWhisperBinary(); binPath != "" {
		w.hasBinary = true
		w.binPath = binPath
	}

	// Check if model exists (ready only if both binary and model exist)
	if _, err := os.Stat(w.modelPath); err == nil && w.hasBinary {
		w.ready = true
		w.setupProgress = 100
	}

	return w, nil
}

func (w *WhisperLocal) Name() string { return "whisper-local" }
func (w *WhisperLocal) DisplayName() string {
	if !w.hasBinary {
		return fmt.Sprintf("Whisper Local (%s) [需安装 whisper.cpp]", w.modelSize)
	}
	return fmt.Sprintf("Whisper Local (%s)", w.modelSize)
}
func (w *WhisperLocal) IsLocal() bool       { return true }
func (w *WhisperLocal) RequiresSetup() bool { return !w.IsReady() }

// HasBinary returns true if whisper-cpp binary is available.
func (w *WhisperLocal) HasBinary() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.hasBinary
}

func (w *WhisperLocal) IsReady() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.ready
}

func (w *WhisperLocal) SetupProgress() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.setupProgress
}

// Setup downloads the whisper model if needed.
func (w *WhisperLocal) Setup(progress func(percent int)) error {
	w.mu.Lock()
	if w.ready {
		w.mu.Unlock()
		return nil
	}
	w.setupProgress = 0
	w.mu.Unlock()

	modelInfo, ok := modelSizes[w.modelSize]
	if !ok {
		return fmt.Errorf("unknown model size: %s", w.modelSize)
	}

	// Create model directory
	modelDir := filepath.Dir(w.modelPath)
	if err := os.MkdirAll(modelDir, 0755); err != nil {
		return fmt.Errorf("create model dir: %w", err)
	}

	// Download model
	if err := w.downloadModel(modelInfo.URL, modelInfo.Size, progress); err != nil {
		return fmt.Errorf("download model: %w", err)
	}

	w.mu.Lock()
	w.ready = true
	w.setupProgress = 100
	w.mu.Unlock()

	if progress != nil {
		progress(100)
	}

	return nil
}

func (w *WhisperLocal) downloadModel(url string, expectedSize int64, progress func(percent int)) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status: %d", resp.StatusCode)
	}

	// Create temp file
	tmpPath := w.modelPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(tmpPath) // Clean up on failure
	}()

	// Download with progress
	var downloaded int64
	buf := make([]byte, 32*1024)
	lastProgress := 0

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write file: %w", werr)
			}
			downloaded += int64(n)

			// Update progress
			if expectedSize > 0 && progress != nil {
				pct := int(downloaded * 100 / expectedSize)
				if pct > lastProgress {
					lastProgress = pct
					w.mu.Lock()
					w.setupProgress = pct
					w.mu.Unlock()
					progress(pct)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}
	}

	// Close file before rename
	if err := f.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	// Move to final location
	if err := os.Rename(tmpPath, w.modelPath); err != nil {
		return fmt.Errorf("rename file: %w", err)
	}

	return nil
}

// Transcribe converts audio samples to text using local whisper.cpp.
// audio: PCM float32 samples at 16000 Hz
// language: source language code (empty for auto-detect)
func (w *WhisperLocal) Transcribe(audio []float32, language string) (*TranscribeResult, error) {
	if !w.IsReady() {
		return nil, fmt.Errorf("WhisperLocal is not ready: model not downloaded")
	}

	// Convert audio to WAV file
	wavData, err := float32ToWAV(audio, 16000)
	if err != nil {
		return nil, fmt.Errorf("convert to WAV: %w", err)
	}

	// Create temp file for audio
	tmpDir := os.TempDir()
	audioPath := filepath.Join(tmpDir, fmt.Sprintf("whisper_audio_%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(audioPath, wavData, 0644); err != nil {
		return nil, fmt.Errorf("write audio file: %w", err)
	}
	defer os.Remove(audioPath)

	// Find or use whisper-cpp binary
	binPath := w.binPath
	if binPath == "" {
		binPath = w.findWhisperBinary()
	}
	if binPath == "" {
		return nil, fmt.Errorf("whisper-cpp binary not found, please install whisper.cpp")
	}

	// Build command
	args := []string{
		"-m", w.modelPath,
		"-f", audioPath,
		"-oj", // Output JSON
		"--no-prints",
	}
	if language != "" {
		args = append(args, "-l", language)
	}

	cmd := exec.Command(binPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("whisper-cpp failed: %w, stderr: %s", err, stderr.String())
	}

	// Parse JSON output
	var whisperOutput whisperCppOutput
	if err := json.Unmarshal(stdout.Bytes(), &whisperOutput); err != nil {
		// Try to parse as plain text if JSON fails
		return &TranscribeResult{
			Text:       stdout.String(),
			Language:   language,
			Confidence: 0.8,
		}, nil
	}

	// Convert to TranscribeResult
	result := &TranscribeResult{
		Text:       "",
		Language:   whisperOutput.Result.Language,
		Confidence: 0.9,
		Segments:   make([]Segment, 0, len(whisperOutput.Transcription)),
	}

	for _, seg := range whisperOutput.Transcription {
		result.Text += seg.Text
		result.Segments = append(result.Segments, Segment{
			Text:  seg.Text,
			Start: time.Duration(seg.Offsets.From) * time.Millisecond * 10, // centiseconds to duration
			End:   time.Duration(seg.Offsets.To) * time.Millisecond * 10,
		})
	}

	return result, nil
}

func (w *WhisperLocal) findWhisperBinary() string {
	// Common binary names - whisper-cli is the Homebrew name
	names := []string{"whisper-cli", "whisper-cpp", "whisper", "main"}

	// Check PATH
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}

	// Check common installation locations
	homeDir, _ := os.UserHomeDir()
	locations := []string{
		"/opt/homebrew/bin",
		"/usr/local/bin",
		filepath.Join(homeDir, ".local", "bin"),
		filepath.Join(homeDir, "whisper.cpp"),
	}

	for _, loc := range locations {
		for _, name := range names {
			path := filepath.Join(loc, name)
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
	}

	// On macOS, check for bundled binary
	if runtime.GOOS == "darwin" {
		// Check in app bundle
		execPath, _ := os.Executable()
		bundlePath := filepath.Join(filepath.Dir(execPath), "..", "Resources", "whisper-cpp")
		if _, err := os.Stat(bundlePath); err == nil {
			return bundlePath
		}
	}

	return ""
}

func (w *WhisperLocal) Close() error {
	return nil
}

// whisperCppOutput represents the JSON output from whisper.cpp.
type whisperCppOutput struct {
	Result struct {
		Language string `json:"language"`
	} `json:"result"`
	Transcription []struct {
		Text    string `json:"text"`
		Offsets struct {
			From int64 `json:"from"`
			To   int64 `json:"to"`
		} `json:"offsets"`
	} `json:"transcription"`
}

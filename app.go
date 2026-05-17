package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// ImageInfo represents info about a single image file
type ImageInfo struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Thumbnail string `json:"thumbnail"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Index     int    `json:"index"`
}

// GifConfig holds all configuration for GIF generation
type GifConfig struct {
	InputFolder  string  `json:"inputFolder"`
	OutputPath   string  `json:"outputPath"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Delay        int     `json:"delay"`
	LoopCount    int     `json:"loopCount"`
	FadeIn       bool    `json:"fadeIn"`
	FadeOut      bool    `json:"fadeOut"`
	FadeDuration float64 `json:"fadeDuration"`
	ScaleMode    string  `json:"scaleMode"`
	Quality      int     `json:"quality"`
	PadColor     string  `json:"padColor"`
}

// App struct
type App struct {
	ctx      context.Context
	progress float64
	mu       sync.Mutex
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SelectFolder opens a native directory picker dialog
func (a *App) SelectFolder() (string, error) {
	selection, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Image Folder",
	})
	if err != nil {
		return "", err
	}
	return selection, nil
}

// SelectOutputFile opens a native save dialog for the output GIF
func (a *App) SelectOutputFile() (string, error) {
	selection, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Save GIF As",
		DefaultFilename: "output.gif",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "GIF Files (*.gif)", Pattern: "*.gif"},
		},
	})
	if err != nil {
		return "", err
	}
	return selection, nil
}

// supportedExtensions lists the image extensions we support
var supportedExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".bmp":  true,
	".webp": true,
	".tiff": true,
	".tif":  true,
}

// GetImages scans a folder and returns info about all supported images
func (a *App) GetImages(folderPath string) ([]ImageInfo, error) {
	if folderPath == "" {
		return nil, fmt.Errorf("no folder selected")
	}

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var images []ImageInfo
	idx := 0

	var fileNames []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if supportedExtensions[ext] {
			fileNames = append(fileNames, entry)
		}
	}

	sort.Slice(fileNames, func(i, j int) bool {
		return strings.ToLower(fileNames[i].Name()) < strings.ToLower(fileNames[j].Name())
	})

	for _, entry := range fileNames {
		fullPath := filepath.Join(folderPath, entry.Name())
		w, h := getImageDimensions(fullPath)
		thumb := generateThumbnailBase64(fullPath)

		images = append(images, ImageInfo{
			Name:      entry.Name(),
			Path:      fullPath,
			Thumbnail: thumb,
			Width:     w,
			Height:    h,
			Index:     idx,
		})
		idx++
	}

	return images, nil
}

func getImageDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func generateThumbnailBase64(path string) string {
	cmd := exec.Command("ffmpeg",
		"-i", path,
		"-vf", "scale=120:-1",
		"-frames:v", "1",
		"-f", "mjpeg",
		"-q:v", "5",
		"pipe:1",
	)
	cmd.Stderr = nil

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(output)
	return "data:image/jpeg;base64," + encoded
}

// GetProgress returns the current conversion progress (0 to 100)
func (a *App) GetProgress() float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.progress
}

func (a *App) setProgress(p float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.progress = p
}

// runFFmpeg runs an ffmpeg command, captures stderr, and returns a meaningful error
func runFFmpeg(args ...string) error {
	logFile, _ := os.OpenFile(filepath.Join(os.TempDir(), "ffmpeg_log.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if logFile != nil {
		defer logFile.Close()
		logFile.WriteString(fmt.Sprintf("\n--- EXEC: ffmpeg %s\n", strings.Join(args, " ")))
	}

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil

	err := cmd.Run()
	
	if logFile != nil {
		logFile.WriteString(fmt.Sprintf("ERROR: %v\nSTDERR:\n%s\n---\n", err, stderr.String()))
	}

	if err != nil {
		stderrStr := stderr.String()
		lines := strings.Split(stderrStr, "\n")
		var errorLines []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if strings.Contains(lower, "error") ||
				strings.Contains(lower, "invalid") ||
				strings.Contains(lower, "no such") ||
				strings.Contains(lower, "not found") ||
				strings.Contains(lower, "nothing was written") {
				errorLines = append(errorLines, trimmed)
			}
		}
		if len(errorLines) > 0 {
			return fmt.Errorf("%s", strings.Join(errorLines, "; "))
		}
		allLines := strings.Split(strings.TrimSpace(stderrStr), "\n")
		var tail []string
		for _, l := range allLines {
			if strings.TrimSpace(l) != "" {
				tail = append(tail, strings.TrimSpace(l))
			}
		}
		if len(tail) > 3 {
			tail = tail[len(tail)-3:]
		}
		return fmt.Errorf("ffmpeg error: %s", strings.Join(tail, " | "))
	}
	return nil
}

// buildScaleFilter builds the ffmpeg scale+pad/crop filter
func buildScaleFilter(width, height int, scaleMode, padColor string) string {
	switch scaleMode {
	case "fit":
		if padColor == "" {
			padColor = "000000"
		}
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease:flags=lanczos,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=#%s",
			width, height, width, height, padColor,
		)
	case "zoom":
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=increase:flags=lanczos,crop=%d:%d",
			width, height, width, height,
		)
	case "stretch":
		return fmt.Sprintf("scale=%d:%d:flags=lanczos", width, height)
	default:
		return fmt.Sprintf(
			"scale=%d:%d:force_original_aspect_ratio=decrease:flags=lanczos,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=#000000",
			width, height, width, height,
		)
	}
}

// GenerateGif creates a GIF from images using FFmpeg.
//
// Pipeline:
//  1. Pre-process each image individually to uniform target size (PNG, rgb24).
//  2. Combine uniform PNGs directly into GIF using filter_complex with
//     optional fade + split + palettegen + paletteuse.
//
// No intermediate video is used. This is the simplest and most reliable pipeline.
func (a *App) GenerateGif(config GifConfig) (string, error) {
	a.setProgress(0)

	if config.InputFolder == "" {
		return "", fmt.Errorf("no input folder specified")
	}

	images, err := a.GetImages(config.InputFolder)
	if err != nil {
		return "", err
	}
	if len(images) == 0 {
		return "", fmt.Errorf("no images found in the selected folder")
	}

	// Output path
	outputPath := config.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(config.InputFolder, "output.gif")
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".gif") {
		outputPath += ".gif"
	}

	// Frame rate from delay
	delay := config.Delay
	if delay < 20 {
		delay = 20
	}
	frameRate := fmt.Sprintf("%g", 1000.0/float64(delay))

	// Dimensions
	width := config.Width
	height := config.Height
	if width <= 0 {
		width = images[0].Width
	}
	if height <= 0 {
		height = images[0].Height
	}
	if width%2 != 0 {
		width++
	}
	if height%2 != 0 {
		height++
	}

	// Create temp working directory
	tmpDir, err := os.MkdirTemp("", "img2gif_")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	a.setProgress(5)

	// Build the scale filter for pre-processing
	scaleFilter := buildScaleFilter(width, height, config.ScaleMode, config.PadColor)
	// format=rgb24 strips alpha channel and ensures consistent pixel format
	frameFilter := scaleFilter + ",format=rgb24"

	// ─── Step 1: Pre-process each image to uniform target size ───
	totalImages := len(images)
	for i, img := range images {
		destPath := filepath.Join(tmpDir, fmt.Sprintf("frame_%05d.png", i))
		err = runFFmpeg(
			"-y",
			"-i", img.Path,
			"-vf", frameFilter,
			"-frames:v", "1",
			destPath,
		)
		if err != nil {
			return "", fmt.Errorf("failed to process image %s: %w", img.Name, err)
		}
		// Progress: 5% to 40%
		a.setProgress(5 + float64(i+1)/float64(totalImages)*35)
	}

	inputPattern := filepath.Join(tmpDir, "frame_%05d.png")

	a.setProgress(45)

	// ─── Step 2: Combine PNGs directly into GIF ───
	quality := config.Quality
	if quality <= 0 || quality > 256 {
		quality = 256
	}
	if quality < 16 {
		quality = 16
	}

	delaySec := float64(delay) / 1000.0
	crossfade := config.FadeIn || config.FadeOut
	fadeDur := config.FadeDuration
	if fadeDur <= 0 {
		fadeDur = 0.5
	}
	// To prevent overlapping fades (which xfade doesn't handle well), limit fade duration
	if fadeDur > delaySec {
		fadeDur = delaySec
	}

	a.setProgress(50)

	var args []string
	args = append(args, "-y")

	if crossfade && totalImages > 1 {
		// PERFECT LOOP CROSSFADE ALGORITHM
		// To make a perfect loop, each input must be shown for (delaySec + fadeDur) seconds.
		// We append the very first image again at the end of the input list.
		inputDur := delaySec + fadeDur

		for i := 0; i <= totalImages; i++ {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%g", inputDur))
			idx := i
			if idx == totalImages {
				idx = 0 // Wrap around to the first image for the final transition
			}
			args = append(args, "-i", filepath.Join(tmpDir, fmt.Sprintf("frame_%05d.png", idx)))
		}

		var fg strings.Builder
		for i := 0; i < totalImages; i++ {
			// Offset increments by exactly delaySec
			offset := delaySec * float64(i+1)
			if i == 0 {
				fg.WriteString(fmt.Sprintf("[0:v][1:v]xfade=transition=fade:duration=%g:offset=%g[v1];", fadeDur, offset))
			} else {
				fg.WriteString(fmt.Sprintf("[v%d][%d:v]xfade=transition=fade:duration=%g:offset=%g[v%d];", i, i+1, fadeDur, offset, i+1))
			}
		}
		
		lastV := fmt.Sprintf("[v%d]", totalImages)
		
		// Trim the video to make the loop seamless:
		// We cut off the first 'fadeDur' seconds (where Image 0 wasn't faded into yet)
		// and we end exactly when the final transition back to Image 0 completes.
		endTime := float64(totalImages)*delaySec + fadeDur
		fg.WriteString(fmt.Sprintf("%strim=start=%g:end=%g,setpts=PTS-STARTPTS[trimmed];", lastV, fadeDur, endTime))
		
		fg.WriteString(fmt.Sprintf("[trimmed]split[s0][s1];[s0]palettegen=max_colors=%d:stats_mode=full[p];[s1][p]paletteuse=dither=sierra2_4a", quality))
		
		args = append(args, "-filter_complex", fg.String())
	} else {
		// Standard image sequence without crossfade
		args = append(args, "-framerate", frameRate)
		args = append(args, "-i", inputPattern)
		filterComplex := fmt.Sprintf("split[s0][s1];[s0]palettegen=max_colors=%d:stats_mode=full[p];[s1][p]paletteuse=dither=sierra2_4a", quality)
		args = append(args, "-filter_complex", filterComplex)
	}

	loopArg := "0"
	if config.LoopCount < 0 {
		loopArg = "-1"
	} else {
		loopArg = fmt.Sprintf("%d", config.LoopCount)
	}
	args = append(args, "-loop", loopArg)
	args = append(args, outputPath)

	err = runFFmpeg(args...)
	if err != nil {
		return "", fmt.Errorf("failed to generate GIF: %w", err)
	}

	a.setProgress(100)

	return outputPath, nil
}

// OpenInExplorer opens the file's parent folder in Windows Explorer
func (a *App) OpenInExplorer(path string) error {
	cmd := exec.Command("explorer", "/select,", path)
	return cmd.Start()
}

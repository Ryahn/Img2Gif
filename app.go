package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/gif"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const thumbnailMaxWidth = 120

var padColorPattern = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

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

	genMu          sync.Mutex
	cancelGenerate context.CancelFunc

	cmdsMu    sync.Mutex
	activeCmd []*exec.Cmd
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	_ = initFFmpegJob()
}

// shutdown cancels in-flight work and terminates tracked FFmpeg processes.
func (a *App) shutdown(ctx context.Context) {
	a.CancelGenerate()
	a.killActiveFFmpeg()
	closeFFmpegJob()
}

// CheckFFmpeg reports whether ffmpeg is available on PATH.
func (a *App) CheckFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// CancelGenerate aborts an in-progress GIF generation.
func (a *App) CancelGenerate() {
	a.genMu.Lock()
	cancel := a.cancelGenerate
	a.cancelGenerate = nil
	a.genMu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.killActiveFFmpeg()
}

func (a *App) registerCmd(cmd *exec.Cmd) {
	a.cmdsMu.Lock()
	a.activeCmd = append(a.activeCmd, cmd)
	a.cmdsMu.Unlock()
}

func (a *App) unregisterCmd(cmd *exec.Cmd) {
	a.cmdsMu.Lock()
	for i, c := range a.activeCmd {
		if c == cmd {
			a.activeCmd = append(a.activeCmd[:i], a.activeCmd[i+1:]...)
			break
		}
	}
	a.cmdsMu.Unlock()
}

func (a *App) killActiveFFmpeg() {
	a.cmdsMu.Lock()
	cmds := append([]*exec.Cmd(nil), a.activeCmd...)
	a.cmdsMu.Unlock()
	for _, cmd := range cmds {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
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

// scanImages lists supported images in a folder (metadata only, no thumbnails).
func scanImages(folderPath string) ([]ImageInfo, error) {
	if folderPath == "" {
		return nil, fmt.Errorf("no folder selected")
	}

	absFolder, err := filepath.Abs(folderPath)
	if err != nil {
		return nil, fmt.Errorf("invalid folder path: %w", err)
	}

	entries, err := os.ReadDir(absFolder)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

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

	var images []ImageInfo
	for idx, entry := range fileNames {
		fullPath, err := resolvePathInDir(absFolder, entry.Name())
		if err != nil {
			continue
		}
		w, h := getImageDimensions(fullPath)
		images = append(images, ImageInfo{
			Name:  entry.Name(),
			Path:  fullPath,
			Width: w,
			Height: h,
			Index: idx,
		})
	}

	return images, nil
}

func resolvePathInDir(dir, name string) (string, error) {
	candidate := filepath.Join(dir, name)
	resolved := candidate
	if link, err := filepath.EvalSymlinks(candidate); err == nil {
		resolved = link
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path escapes folder: %s", name)
	}
	return absPath, nil
}

// GetImages scans a folder and returns info about all supported images (with thumbnails).
func (a *App) GetImages(folderPath string) ([]ImageInfo, error) {
	images, err := scanImages(folderPath)
	if err != nil {
		return nil, err
	}

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	for i := range images {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		images[i].Thumbnail = generateThumbnailBase64(images[i].Path)
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

func thumbnailSize(src image.Image) (int, int) {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0
	}
	if w <= thumbnailMaxWidth {
		return w, h
	}
	nw := thumbnailMaxWidth
	nh := h * nw / w
	if nh < 1 {
		nh = 1
	}
	return nw, nh
}

func generateThumbnailBase64(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	src, _, err := image.Decode(f)
	if err != nil {
		return ""
	}

	tw, th := thumbnailSize(src)
	if tw == 0 || th == 0 {
		return ""
	}

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 75}); err != nil {
		return ""
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
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

func normalizePadColor(raw string) (string, error) {
	color := strings.TrimPrefix(strings.TrimSpace(raw), "#")
	if color == "" {
		return "000000", nil
	}
	if !padColorPattern.MatchString(color) {
		return "", fmt.Errorf("pad color must be a 6-digit hex value")
	}
	return strings.ToLower(color), nil
}

// runFFmpeg runs an ffmpeg command with cancellation support.
func (a *App) runFFmpeg(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = nil

	a.registerCmd(cmd)
	defer a.unregisterCmd(cmd)

	if err := cmd.Start(); err != nil {
		return err
	}
	_ = assignChildToFFmpegJob(cmd.Process.Pid)

	err := cmd.Wait()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
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

// escapeConcatPath quotes a path for use in an ffconcat list file.
func escapeConcatPath(path string) string {
	path = filepath.ToSlash(path)
	return strings.ReplaceAll(path, "'", `'\''`)
}

// writeConcatList writes an ffconcat manifest for one-frame-per-file image inputs.
func writeConcatList(listPath string, images []ImageInfo) error {
	var b strings.Builder
	b.WriteString("ffconcat version 1.0\n")
	for _, img := range images {
		b.WriteString("file '")
		b.WriteString(escapeConcatPath(img.Path))
		b.WriteString("'\n")
	}
	return os.WriteFile(listPath, []byte(b.String()), 0600)
}

// preprocessFrames scales all source images to uniform PNG frames in one FFmpeg run.
func (a *App) preprocessFrames(ctx context.Context, images []ImageInfo, tmpDir, frameFilter string) error {
	listPath := filepath.Join(tmpDir, "inputs.ffconcat")
	if err := writeConcatList(listPath, images); err != nil {
		return fmt.Errorf("failed to write concat list: %w", err)
	}

	outputPattern := filepath.Join(tmpDir, "frame_%05d.png")
	return a.runFFmpeg(ctx,
		"-y",
		"-f", "concat",
		"-safe", "0",
		"-i", listPath,
		"-vf", frameFilter,
		"-vsync", "0",
		"-start_number", "0",
		outputPattern,
	)
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
func (a *App) GenerateGif(config GifConfig) (string, error) {
	a.genMu.Lock()
	if a.cancelGenerate != nil {
		a.genMu.Unlock()
		return "", fmt.Errorf("GIF generation already in progress")
	}
	genCtx, cancel := context.WithCancel(a.ctx)
	a.cancelGenerate = cancel
	a.genMu.Unlock()

	defer func() {
		cancel()
		a.genMu.Lock()
		a.cancelGenerate = nil
		a.genMu.Unlock()
	}()

	a.setProgress(0)

	if config.InputFolder == "" {
		return "", fmt.Errorf("no input folder specified")
	}

	padColor, err := normalizePadColor(config.PadColor)
	if err != nil {
		return "", err
	}
	config.PadColor = padColor

	images, err := scanImages(config.InputFolder)
	if err != nil {
		return "", err
	}
	if len(images) == 0 {
		return "", fmt.Errorf("no images found in the selected folder")
	}

	outputPath := config.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(config.InputFolder, "output.gif")
	}
	if !strings.HasSuffix(strings.ToLower(outputPath), ".gif") {
		outputPath += ".gif"
	}

	delay := config.Delay
	if delay < 20 {
		delay = 20
	}
	frameRate := fmt.Sprintf("%g", 1000.0/float64(delay))

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

	tmpDir, err := os.MkdirTemp("", "img2gif_")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := genCtx.Err(); err != nil {
		return "", err
	}

	a.setProgress(5)

	scaleFilter := buildScaleFilter(width, height, config.ScaleMode, config.PadColor)
	frameFilter := scaleFilter + ",format=rgb24"

	totalImages := len(images)
	if err := a.preprocessFrames(genCtx, images, tmpDir, frameFilter); err != nil {
		if genCtx.Err() != nil {
			return "", errors.New("GIF generation cancelled")
		}
		return "", fmt.Errorf("failed to preprocess images: %w", err)
	}
	a.setProgress(40)

	inputPattern := filepath.Join(tmpDir, "frame_%05d.png")

	a.setProgress(45)

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
	if fadeDur > delaySec {
		fadeDur = delaySec
	}

	a.setProgress(50)

	var args []string
	args = append(args, "-y")

	if crossfade && totalImages > 1 {
		inputDur := delaySec + fadeDur

		for i := 0; i <= totalImages; i++ {
			args = append(args, "-loop", "1", "-t", fmt.Sprintf("%g", inputDur))
			idx := i
			if idx == totalImages {
				idx = 0
			}
			args = append(args, "-i", filepath.Join(tmpDir, fmt.Sprintf("frame_%05d.png", idx)))
		}

		var fg strings.Builder
		for i := 0; i < totalImages; i++ {
			offset := delaySec * float64(i+1)
			if i == 0 {
				fg.WriteString(fmt.Sprintf("[0:v][1:v]xfade=transition=fade:duration=%g:offset=%g[v1];", fadeDur, offset))
			} else {
				fg.WriteString(fmt.Sprintf("[v%d][%d:v]xfade=transition=fade:duration=%g:offset=%g[v%d];", i, i+1, fadeDur, offset, i+1))
			}
		}

		lastV := fmt.Sprintf("[v%d]", totalImages)
		endTime := float64(totalImages)*delaySec + fadeDur
		fg.WriteString(fmt.Sprintf("%strim=start=%g:end=%g,setpts=PTS-STARTPTS[trimmed];", lastV, fadeDur, endTime))
		fg.WriteString(fmt.Sprintf("[trimmed]split[s0][s1];[s0]palettegen=max_colors=%d:stats_mode=full[p];[s1][p]paletteuse=dither=sierra2_4a", quality))

		args = append(args, "-filter_complex", fg.String())
	} else {
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

	err = a.runFFmpeg(genCtx, args...)
	if err != nil {
		if genCtx.Err() != nil {
			return "", errors.New("GIF generation cancelled")
		}
		return "", fmt.Errorf("failed to generate GIF: %w", err)
	}

	a.setProgress(100)
	return outputPath, nil
}

// OpenInExplorer opens the file's parent folder in Windows Explorer
func (a *App) OpenInExplorer(path string) error {
	arg := "/select," + path
	if strings.ContainsAny(path, ", ") {
		arg = `/select,"` + path + `"`
	}
	cmd := exec.Command("explorer.exe", arg)
	return cmd.Start()
}

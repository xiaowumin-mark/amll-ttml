package ttml

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type extremeFileLog struct {
	InputPath              string  `json:"input_path"`
	BinaryPath             string  `json:"binary_path,omitempty"`
	RoundTripTTMLPath      string  `json:"roundtrip_ttml_path,omitempty"`
	InputSizeBytes         int     `json:"input_size_bytes"`
	BinarySizeBytes        int     `json:"binary_size_bytes"`
	RoundTripTTMLSizeBytes int     `json:"roundtrip_ttml_size_bytes"`
	TTMLToBinaryMs         float64 `json:"ttml_to_binary_ms"`
	BinaryToTTMLMs         float64 `json:"binary_to_ttml_ms"`
	TotalMs                float64 `json:"total_ms"`
	Success                bool    `json:"success"`
	Error                  string  `json:"error,omitempty"`
}

type extremeSummary struct {
	StartedAtUTC       string  `json:"started_at_utc"`
	FinishedAtUTC      string  `json:"finished_at_utc"`
	ElapsedMs          float64 `json:"elapsed_ms"`
	InputDir           string  `json:"input_dir"`
	BinaryOutputDir    string  `json:"binary_output_dir"`
	RoundTripOutputDir string  `json:"roundtrip_output_dir"`
	TotalFiles         int     `json:"total_files"`
	SuccessFiles       int     `json:"success_files"`
	FailedFiles        int     `json:"failed_files"`
	AvgTTMLToBinaryMs  float64 `json:"avg_ttml_to_binary_ms"`
	AvgBinaryToTTMLMs  float64 `json:"avg_binary_to_ttml_ms"`
	AvgTotalMs         float64 `json:"avg_total_ms"`
	LogTextPath        string  `json:"log_text_path"`
	LogJSONPath        string  `json:"log_json_path"`
}

type extremeReport struct {
	Summary extremeSummary   `json:"summary"`
	Files   []extremeFileLog `json:"files"`
}

func TestExtremeTTMLBinaryPipeline(t *testing.T) {
	if os.Getenv("RUN_EXTREME_TEST") != "1" {
		t.Skip("set RUN_EXTREME_TEST=1 to run this extreme test")
	}

	testRootDir := filepath.Join("test")
	inputDir := filepath.Join(testRootDir, "raw-ttml")
	binaryOutputDir := filepath.Join(testRootDir, "binary")
	roundTripOutputDir := filepath.Join(testRootDir, "binary-to-ttml")
	logTextPath := filepath.Join(testRootDir, "extreme-conversion.log")
	logJSONPath := filepath.Join(testRootDir, "extreme-conversion.json")

	if err := os.MkdirAll(testRootDir, 0o755); err != nil {
		t.Fatalf("create test root dir: %v", err)
	}
	if err := os.RemoveAll(binaryOutputDir); err != nil {
		t.Fatalf("cleanup binary output dir: %v", err)
	}
	if err := os.RemoveAll(roundTripOutputDir); err != nil {
		t.Fatalf("cleanup round-trip output dir: %v", err)
	}
	if err := os.MkdirAll(binaryOutputDir, 0o755); err != nil {
		t.Fatalf("create binary output dir: %v", err)
	}
	if err := os.MkdirAll(roundTripOutputDir, 0o755); err != nil {
		t.Fatalf("create round-trip output dir: %v", err)
	}

	inputFiles, err := collectTTMLFiles(inputDir)
	if err != nil {
		t.Fatalf("collect input files: %v", err)
	}
	if len(inputFiles) == 0 {
		t.Fatalf("no .ttml files found under %s", inputDir)
	}

	startedAt := time.Now().UTC()
	start := time.Now()
	fileLogs := make([]extremeFileLog, 0, len(inputFiles))

	var sumTTMLToBinary time.Duration
	var sumBinaryToTTML time.Duration
	var successCount int

	for _, inputPath := range inputFiles {
		relativePath, err := filepath.Rel(inputDir, inputPath)
		if err != nil {
			relativePath = inputPath
		}

		fileLog := extremeFileLog{
			InputPath: relativePath,
		}

		rawTTML, err := os.ReadFile(inputPath)
		if err != nil {
			fileLog.Error = fmt.Sprintf("read input file: %v", err)
			fileLogs = append(fileLogs, fileLog)
			continue
		}
		fileLog.InputSizeBytes = len(rawTTML)

		ttmlToBinaryStart := time.Now()
		binaryData, err := TTMLToBinary(string(rawTTML))
		ttmlToBinaryDuration := time.Since(ttmlToBinaryStart)
		fileLog.TTMLToBinaryMs = durationToMS(ttmlToBinaryDuration)
		if err != nil {
			fileLog.Error = fmt.Sprintf("TTMLToBinary: %v", err)
			fileLog.TotalMs = fileLog.TTMLToBinaryMs
			fileLogs = append(fileLogs, fileLog)
			continue
		}

		binaryRelativePath := replaceExt(relativePath, ".amlx")
		binaryPath := filepath.Join(binaryOutputDir, binaryRelativePath)
		if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
			fileLog.Error = fmt.Sprintf("create binary output dir: %v", err)
			fileLog.TotalMs = fileLog.TTMLToBinaryMs
			fileLogs = append(fileLogs, fileLog)
			continue
		}
		if err := os.WriteFile(binaryPath, binaryData, 0o644); err != nil {
			fileLog.Error = fmt.Sprintf("write binary output: %v", err)
			fileLog.TotalMs = fileLog.TTMLToBinaryMs
			fileLogs = append(fileLogs, fileLog)
			continue
		}
		fileLog.BinaryPath = binaryRelativePath
		fileLog.BinarySizeBytes = len(binaryData)

		binaryToTTMLStart := time.Now()
		roundTripTTML, err := BinaryToTTML(binaryData, false)
		binaryToTTMLDuration := time.Since(binaryToTTMLStart)
		fileLog.BinaryToTTMLMs = durationToMS(binaryToTTMLDuration)
		if err != nil {
			fileLog.Error = fmt.Sprintf("BinaryToTTML: %v", err)
			fileLog.TotalMs = fileLog.TTMLToBinaryMs + fileLog.BinaryToTTMLMs
			fileLogs = append(fileLogs, fileLog)
			continue
		}

		roundTripRelativePath := replaceExt(relativePath, ".ttml")
		roundTripPath := filepath.Join(roundTripOutputDir, roundTripRelativePath)
		if err := os.MkdirAll(filepath.Dir(roundTripPath), 0o755); err != nil {
			fileLog.Error = fmt.Sprintf("create round-trip output dir: %v", err)
			fileLog.TotalMs = fileLog.TTMLToBinaryMs + fileLog.BinaryToTTMLMs
			fileLogs = append(fileLogs, fileLog)
			continue
		}
		if err := os.WriteFile(roundTripPath, []byte(roundTripTTML), 0o644); err != nil {
			fileLog.Error = fmt.Sprintf("write round-trip ttml: %v", err)
			fileLog.TotalMs = fileLog.TTMLToBinaryMs + fileLog.BinaryToTTMLMs
			fileLogs = append(fileLogs, fileLog)
			continue
		}
		fileLog.RoundTripTTMLPath = roundTripRelativePath
		fileLog.RoundTripTTMLSizeBytes = len(roundTripTTML)

		fileLog.TotalMs = fileLog.TTMLToBinaryMs + fileLog.BinaryToTTMLMs
		fileLog.Success = true
		fileLogs = append(fileLogs, fileLog)

		sumTTMLToBinary += ttmlToBinaryDuration
		sumBinaryToTTML += binaryToTTMLDuration
		successCount++
	}

	elapsed := time.Since(start)
	failedCount := len(fileLogs) - successCount

	avgTTMLToBinaryMs := 0.0
	avgBinaryToTTMLMs := 0.0
	avgTotalMs := 0.0
	if successCount > 0 {
		avgTTMLToBinaryMs = durationToMS(sumTTMLToBinary) / float64(successCount)
		avgBinaryToTTMLMs = durationToMS(sumBinaryToTTML) / float64(successCount)
		avgTotalMs = avgTTMLToBinaryMs + avgBinaryToTTMLMs
	}

	report := extremeReport{
		Summary: extremeSummary{
			StartedAtUTC:       startedAt.Format(time.RFC3339Nano),
			FinishedAtUTC:      time.Now().UTC().Format(time.RFC3339Nano),
			ElapsedMs:          durationToMS(elapsed),
			InputDir:           inputDir,
			BinaryOutputDir:    binaryOutputDir,
			RoundTripOutputDir: roundTripOutputDir,
			TotalFiles:         len(fileLogs),
			SuccessFiles:       successCount,
			FailedFiles:        failedCount,
			AvgTTMLToBinaryMs:  avgTTMLToBinaryMs,
			AvgBinaryToTTMLMs:  avgBinaryToTTMLMs,
			AvgTotalMs:         avgTotalMs,
			LogTextPath:        logTextPath,
			LogJSONPath:        logJSONPath,
		},
		Files: fileLogs,
	}

	if err := os.WriteFile(logTextPath, []byte(renderExtremeTextLog(report)), 0o644); err != nil {
		t.Fatalf("write text log: %v", err)
	}

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal json log: %v", err)
	}
	if err := os.WriteFile(logJSONPath, jsonBytes, 0o644); err != nil {
		t.Fatalf("write json log: %v", err)
	}

	t.Logf("extreme test finished: total=%d success=%d failed=%d avg_ttml_to_binary_ms=%.3f avg_binary_to_ttml_ms=%.3f",
		report.Summary.TotalFiles, report.Summary.SuccessFiles, report.Summary.FailedFiles,
		report.Summary.AvgTTMLToBinaryMs, report.Summary.AvgBinaryToTTMLMs)
	t.Logf("logs: %s, %s", logTextPath, logJSONPath)

	if failedCount > 0 {
		t.Fatalf("extreme test has %d failed files, see %s", failedCount, logTextPath)
	}
}

func collectTTMLFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".ttml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func replaceExt(path, newExt string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path + newExt
	}
	return strings.TrimSuffix(path, ext) + newExt
}

func renderExtremeTextLog(report extremeReport) string {
	var sb strings.Builder

	s := report.Summary
	sb.WriteString("AMLX Extreme Conversion Report\n")
	sb.WriteString(fmt.Sprintf("StartedAtUTC: %s\n", s.StartedAtUTC))
	sb.WriteString(fmt.Sprintf("FinishedAtUTC: %s\n", s.FinishedAtUTC))
	sb.WriteString(fmt.Sprintf("ElapsedMs: %.3f\n", s.ElapsedMs))
	sb.WriteString(fmt.Sprintf("InputDir: %s\n", s.InputDir))
	sb.WriteString(fmt.Sprintf("BinaryOutputDir: %s\n", s.BinaryOutputDir))
	sb.WriteString(fmt.Sprintf("RoundTripOutputDir: %s\n", s.RoundTripOutputDir))
	sb.WriteString(fmt.Sprintf("TotalFiles: %d\n", s.TotalFiles))
	sb.WriteString(fmt.Sprintf("SuccessFiles: %d\n", s.SuccessFiles))
	sb.WriteString(fmt.Sprintf("FailedFiles: %d\n", s.FailedFiles))
	sb.WriteString(fmt.Sprintf("AvgTTMLToBinaryMs: %.3f\n", s.AvgTTMLToBinaryMs))
	sb.WriteString(fmt.Sprintf("AvgBinaryToTTMLMs: %.3f\n", s.AvgBinaryToTTMLMs))
	sb.WriteString(fmt.Sprintf("AvgTotalMs: %.3f\n", s.AvgTotalMs))
	sb.WriteString("\nPerFile:\n")
	for _, f := range report.Files {
		if f.Success {
			sb.WriteString(fmt.Sprintf(
				"OK | %s | ttml->binary=%.3fms | binary->ttml=%.3fms | total=%.3fms | input=%dB | binary=%dB | output=%dB\n",
				f.InputPath, f.TTMLToBinaryMs, f.BinaryToTTMLMs, f.TotalMs, f.InputSizeBytes, f.BinarySizeBytes, f.RoundTripTTMLSizeBytes,
			))
			continue
		}
		sb.WriteString(fmt.Sprintf(
			"FAIL | %s | ttml->binary=%.3fms | binary->ttml=%.3fms | total=%.3fms | error=%s\n",
			f.InputPath, f.TTMLToBinaryMs, f.BinaryToTTMLMs, f.TotalMs, f.Error,
		))
	}

	return sb.String()
}

func durationToMS(d time.Duration) float64 {
	return float64(d) / float64(time.Millisecond)
}

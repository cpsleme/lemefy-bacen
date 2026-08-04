package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadFile(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "test.txt")

	err := DownloadFile("https://www.example.com", dest)
	if err != nil {
		t.Logf("DownloadFile failed (expected for test URL): %v", err)
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	existingFile := filepath.Join(tmpDir, "exists.txt")
	nonExistingFile := filepath.Join(tmpDir, "not_exists.txt")

	err := os.WriteFile(existingFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	if !FileExists(existingFile) {
		t.Error("expected existing file to exist")
	}
	if FileExists(nonExistingFile) {
		t.Error("expected non-existing file to not exist")
	}
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()
	newDir := filepath.Join(tmpDir, "new", "nested", "dir")

	err := EnsureDir(newDir)
	if err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	info, err := os.Stat(newDir)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected path to be a directory")
	}
}

func TestReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := "hello world"

	err := os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	readContent, err := ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if readContent != content {
		t.Errorf("expected %q, got %q", content, readContent)
	}
}

func TestReadFileNotFound(t *testing.T) {
	_, err := ReadFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestWriteFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")
	content := "test content"

	err := WriteFile(filePath, content)
	if err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	readContent, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(readContent) != content {
		t.Errorf("expected %q, got %q", content, string(readContent))
	}
}

func TestPrettyJSON(t *testing.T) {
	data := map[string]interface{}{
		"key": "value",
		"number": 42,
	}

	result, err := PrettyJSON(data)
	if err != nil {
		t.Fatalf("PrettyJSON failed: %v", err)
	}

	if result == "" {
		t.Error("PrettyJSON returned empty string")
	}
}

func TestCompactJSON(t *testing.T) {
	data := map[string]interface{}{
		"key": "value",
		"number": 42,
	}

	result, err := CompactJSON(data)
	if err != nil {
		t.Fatalf("CompactJSON failed: %v", err)
	}

	if result == "" {
		t.Error("CompactJSON returned empty string")
	}
}

func TestParseDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"ISO format", "2024-01-15", false},
		{"Brazilian format", "15/01/2024", false},
		{"With time", "2024-01-15 10:30:00", false},
		{"Empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseDate(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for input %q, got nil", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestCleanText(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{"multiple spaces", "hello    world", "hello world"},
		{"tabs", "hello\tworld", "hello world"},
		{"newlines", "hello\nworld", "hello world"},
		{"leading/trailing", "  hello  ", "hello"},
		{"non-breaking space", "hello\u00a0world", "hello world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanText(tt.input)
			if got != tt.want {
				t.Errorf("CleanText(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name string
		input string
		want string
	}{
		{"normal", "file.txt", "file.txt"},
		{"with slash", "dir/file.txt", "dir_file.txt"},
		{"with colon", "file:name.txt", "file-name.txt"},
		{"with asterisk", "file*.txt", "file.txt"},
		{"with question", "file?.txt", "file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("TEST_KEY", "test_value")

	val := GetEnv("TEST_KEY", "default")
	if val != "test_value" {
		t.Errorf("expected 'test_value', got %s", val)
	}

	val = GetEnv("NONEXISTENT_KEY", "default")
	if val != "default" {
		t.Errorf("expected 'default', got %s", val)
	}
}

func TestGetEnvInt(t *testing.T) {
	t.Setenv("TEST_INT", "42")

	val := GetEnvInt("TEST_INT", 0)
	if val != 42 {
		t.Errorf("expected 42, got %d", val)
	}

	val = GetEnvInt("NONEXISTENT_INT", 10)
	if val != 10 {
		t.Errorf("expected 10, got %d", val)
	}
}

func TestGetEnvBool(t *testing.T) {
	t.Setenv("TEST_BOOL", "true")

	val := GetEnvBool("TEST_BOOL", false)
	if !val {
		t.Error("expected true")
	}

	val = GetEnvBool("NONEXISTENT_BOOL", true)
	if !val {
		t.Error("expected true as default")
	}
}
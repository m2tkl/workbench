package taskbench

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceWorkbenchUploadStagesFile(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := io.WriteString(part, "alpha\nbeta\n"); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close returned error: %v", err)
	}

	uploadReq := httptest.NewRequest(http.MethodPost, "/upload", body)
	uploadReq.Header.Set("Content-Type", writer.FormDataContentType())
	uploadRes := httptest.NewRecorder()
	server.routes().ServeHTTP(uploadRes, uploadReq)
	if uploadRes.Code != http.StatusSeeOther {
		t.Fatalf("upload status = %d, want %d", uploadRes.Code, http.StatusSeeOther)
	}
	if _, err := os.Stat(filepath.Join(vault.SourceStagedDir(), "notes.txt")); err != nil {
		t.Fatalf("expected staged file to exist: %v", err)
	}
	files, err := os.ReadDir(vault.SourceDocumentsDir())
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("SourceDocumentsDir len = %d, want 0", len(files))
	}
}

func TestSourceWorkbenchIndexShowsStagedFiles(t *testing.T) {
	root := t.TempDir()
	vault := NewVault(root)
	if err := vault.EnsureLayout(); err != nil {
		t.Fatalf("EnsureLayout returned error: %v", err)
	}
	if _, err := vault.StageSourceUpload("deck.pptx", strings.NewReader("fake")); err != nil {
		t.Fatalf("StageSourceUpload returned error: %v", err)
	}

	server := newSourceWorkbenchServer(vault)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	server.routes().ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("index status = %d, want %d", res.Code, http.StatusOK)
	}
	body := res.Body.String()
	if !strings.Contains(body, "deck.pptx") {
		t.Fatalf("expected staged filename in body: %s", body)
	}
	if !strings.Contains(body, "Extract this later with an agent or CLI flow.") {
		t.Fatalf("expected staged guidance in body: %s", body)
	}
	if !strings.Contains(body, "Source Inbox") {
		t.Fatalf("expected page title in body: %s", body)
	}
}

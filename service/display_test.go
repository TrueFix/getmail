package service

import (
	"io"
	"strings"
	"testing"

	"github.com/TrueFix/getmail/email"
)

func TestPreviewPreservesBody(t *testing.T) {
	content := &email.EmailContent{R: strings.NewReader(strings.Repeat("x", 150))}
	preview, err := previewBody(content)
	if err != nil || len(preview) != 100 {
		t.Fatalf("preview length=%d err=%v", len(preview), err)
	}
	full, err := io.ReadAll(content)
	if err != nil || len(full) != 150 {
		t.Fatalf("preview consumed message body: %d bytes left, err=%v", len(full), err)
	}
}

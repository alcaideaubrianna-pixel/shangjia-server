package file

import (
	"io"
	"os"
	"testing"
)

func TestNewMultipartFileHeaderFromPathStreamsFile(t *testing.T) {
	input, err := os.CreateTemp(t.TempDir(), "collect-media-*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	content := make([]byte, 2*1024*1024)
	for index := range content {
		content[index] = byte(index % 251)
	}
	if _, err = input.Write(content); err != nil {
		t.Fatal(err)
	}
	if err = input.Close(); err != nil {
		t.Fatal(err)
	}

	header, cleanup, err := NewMultipartFileHeaderFromPath(input.Name(), "sample.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if header.Filename != "sample.jpg" || header.Size != int64(len(content)) {
		t.Fatalf("unexpected header: filename=%s size=%d", header.Filename, header.Size)
	}
	reader, err := header.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err = io.Copy(io.Discard, reader); err != nil {
		t.Fatal(err)
	}
}

package api

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"testing"
)

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 80, 80))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func checkTemplateUpload(t *testing.T, withFilename bool) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	var part interface{ Write([]byte) (int, error) }
	var err error
	if withFilename {
		part, err = w.CreateFormFile("image", "pasted.png")
	} else {
		// A clipboard image without a filename is written by CreateFormField.
		part, err = w.CreateFormField("image")
	}
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(pngBytes(t)); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/api/templates", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if err := req.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("parse multipart: %v", err)
	}

	data, cfg, format, err := readTemplateUpload(req)
	if err != nil {
		t.Fatalf("readTemplateUpload: %v", err)
	}
	if len(data) == 0 || cfg.Width != 80 || cfg.Height != 80 || format != "png" {
		t.Fatalf("upload = %d bytes, %dx%d %s", len(data), cfg.Width, cfg.Height, format)
	}
}

func TestReadTemplateUpload(t *testing.T) {
	t.Run("file part", func(t *testing.T) { checkTemplateUpload(t, true) })
	t.Run("clipboard image without filename", func(t *testing.T) { checkTemplateUpload(t, false) })
}

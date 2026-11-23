package attachments

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func createSampleImageBytes(t *testing.T, format string, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 255, A: 255})
		}
	}

	var buf bytes.Buffer
	if format == "jpeg" {
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			t.Fatalf("failed to encode sample jpeg: %v", err)
		}
	} else if format == "png" {
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("failed to encode sample png: %v", err)
		}
	}
	return buf.Bytes()
}

func TestProcessAndNormalizeImage_ValidJPEG(t *testing.T) {
	raw := createSampleImageBytes(t, "jpeg", 100, 100)
	proc, err := ProcessAndNormalizeImage(bytes.NewReader(raw), "photo.jpg")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if proc.MIMEType != "image/jpeg" {
		t.Errorf("expected image/jpeg, got %s", proc.MIMEType)
	}
	if proc.Width != 100 || proc.Height != 100 {
		t.Errorf("expected 100x100, got %dx%d", proc.Width, proc.Height)
	}
	if proc.ByteSize <= 0 {
		t.Errorf("expected positive byte size, got %d", proc.ByteSize)
	}
}

func TestProcessAndNormalizeImage_ValidPNG(t *testing.T) {
	raw := createSampleImageBytes(t, "png", 50, 50)
	proc, err := ProcessAndNormalizeImage(bytes.NewReader(raw), "screenshot.png")
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if proc.MIMEType != "image/png" {
		t.Errorf("expected image/png, got %s", proc.MIMEType)
	}
	if proc.Width != 50 || proc.Height != 50 {
		t.Errorf("expected 50x50, got %dx%d", proc.Width, proc.Height)
	}
}

func TestProcessAndNormalizeImage_MIMEMismatch(t *testing.T) {
	fakeBytes := []byte("<html><body>Fake Image Script</body></html>")
	_, err := ProcessAndNormalizeImage(bytes.NewReader(fakeBytes), "fake.jpg")
	if err == nil {
		t.Fatalf("expected error for MIME mismatch/fake image, got nil")
	}
	if !errors.Is(err, ErrUnsupportedMediaType) && !errors.Is(err, ErrInvalidImageData) {
		t.Errorf("expected ErrUnsupportedMediaType or ErrInvalidImageData, got: %v", err)
	}
}

func TestProcessAndNormalizeImage_OversizedFile(t *testing.T) {
	oversizedBytes := make([]byte, MaxFileBytes+100)
	copy(oversizedBytes, []byte{0xFF, 0xD8, 0xFF, 0xE0})

	_, err := ProcessAndNormalizeImage(bytes.NewReader(oversizedBytes), "large.jpg")
	if err == nil {
		t.Fatalf("expected ErrFileTooLarge, got nil")
	}
	if !errors.Is(err, ErrFileTooLarge) {
		t.Errorf("expected ErrFileTooLarge, got: %v", err)
	}
}

func TestProcessAndNormalizeImage_DimensionsExceeded(t *testing.T) {
	raw := createSampleImageBytes(t, "png", 4097, 10)
	_, err := ProcessAndNormalizeImage(bytes.NewReader(raw), "too_wide.png")
	if err == nil {
		t.Fatalf("expected ErrDimensionsExceeded, got nil")
	}
	if !errors.Is(err, ErrDimensionsExceeded) {
		t.Errorf("expected ErrDimensionsExceeded, got: %v", err)
	}
}

func TestProcessAndNormalizeImage_CorruptedFile(t *testing.T) {
	raw := createSampleImageBytes(t, "jpeg", 100, 100)
	truncated := raw[:20]

	_, err := ProcessAndNormalizeImage(bytes.NewReader(truncated), "corrupt.jpg")
	if err == nil {
		t.Fatalf("expected error for corrupted image, got nil")
	}
}

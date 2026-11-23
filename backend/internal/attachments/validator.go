package attachments

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	_ "golang.org/x/image/webp"
)

const (
	MaxFileBytes = 5 * 1024 * 1024 // 5 MiB
	MaxWidth     = 4096
	MaxHeight    = 4096
	MaxPixels    = 16 * 1000 * 1000 // 16 Megapixels
)

var (
	ErrFileTooLarge         = errors.New("file size exceeds maximum limit of 5 MiB")
	ErrUnsupportedMediaType = errors.New("unsupported image type; only JPEG, PNG, and WebP are allowed")
	ErrDimensionsExceeded   = errors.New("image dimensions exceed maximum allowed limits (4096x4096 or 16 MP)")
	ErrInvalidImageData     = errors.New("malformed or corrupted image data")
)

type ProcessedImage struct {
	Data     []byte
	MIMEType string
	Format   string
	Width    int
	Height   int
	ByteSize int64
}

// ProcessAndNormalizeImage validates bytes, decodes image, checks size/dimensions,
// strips EXIF/metadata, and re-encodes into normalized binary data.
func ProcessAndNormalizeImage(r io.Reader, originalFilename string) (*ProcessedImage, error) {
	lr := io.LimitReader(r, MaxFileBytes+1)
	rawBytes, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidImageData, err)
	}

	if len(rawBytes) > MaxFileBytes {
		return nil, ErrFileTooLarge
	}
	if len(rawBytes) == 0 {
		return nil, fmt.Errorf("%w: empty file", ErrInvalidImageData)
	}

	sniffSize := 512
	if len(rawBytes) < sniffSize {
		sniffSize = len(rawBytes)
	}
	detectedMIME := http.DetectContentType(rawBytes[:sniffSize])

	switch detectedMIME {
	case "image/jpeg", "image/png", "image/webp":
		// Allowed content types
	default:
		return nil, fmt.Errorf("%w: detected %s", ErrUnsupportedMediaType, detectedMIME)
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(rawBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode image header: %v", ErrInvalidImageData, err)
	}

	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, fmt.Errorf("%w: invalid image dimensions", ErrInvalidImageData)
	}

	if cfg.Width > MaxWidth || cfg.Height > MaxHeight {
		return nil, fmt.Errorf("%w: width %d or height %d exceeds limit of %dx%d", ErrDimensionsExceeded, cfg.Width, cfg.Height, MaxWidth, MaxHeight)
	}

	pixels := int64(cfg.Width) * int64(cfg.Height)
	if pixels > MaxPixels {
		return nil, fmt.Errorf("%w: total pixels %d exceed 16 MP limit", ErrDimensionsExceeded, pixels)
	}

	img, _, err := image.Decode(bytes.NewReader(rawBytes))
	if err != nil {
		return nil, fmt.Errorf("%w: failed to decode image pixels: %v", ErrInvalidImageData, err)
	}

	var buf bytes.Buffer
	var outputMIME string
	var outputFormat string

	switch format {
	case "png":
		encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("%w: failed to re-encode PNG: %v", ErrInvalidImageData, err)
		}
		outputMIME = "image/png"
		outputFormat = "png"
	case "jpeg", "jpg":
		opts := &jpeg.Options{Quality: 85}
		if err := jpeg.Encode(&buf, img, opts); err != nil {
			return nil, fmt.Errorf("%w: failed to re-encode JPEG: %v", ErrInvalidImageData, err)
		}
		outputMIME = "image/jpeg"
		outputFormat = "jpeg"
	case "webp":
		encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("%w: failed to re-encode WebP image: %v", ErrInvalidImageData, err)
		}
		outputMIME = "image/png"
		outputFormat = "png"
	default:
		return nil, ErrUnsupportedMediaType
	}

	normalizedData := buf.Bytes()
	return &ProcessedImage{
		Data:     normalizedData,
		MIMEType: outputMIME,
		Format:   outputFormat,
		Width:    cfg.Width,
		Height:   cfg.Height,
		ByteSize: int64(len(normalizedData)),
	}, nil
}

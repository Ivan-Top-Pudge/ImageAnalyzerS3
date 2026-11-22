package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"uuid"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/config"
)

type recordingService struct {
	uploads int
	data    []byte
	id      uuid.UUID
	err     error
}

func (s *recordingService) Upload(_ context.Context, data []byte) (uuid.UUID, error) {
	s.uploads++
	s.data = data
	return s.id, s.err
}

type discardLogger struct{}

func (discardLogger) Log(string) {}

func TestUpload(t *testing.T) {
	jpegImage := encodeImage(t, 4, func(w io.Writer, img image.Image) error { return jpeg.Encode(w, img, nil) })
	pngImage := encodeImage(t, 4, png.Encode)
	largePNG := encodeImage(t, 5, png.Encode)
	gifImage := encodeImage(t, 4, func(w io.Writer, img image.Image) error {
		return gif.Encode(w, img, &gif.Options{NumColors: 2})
	})
	limits := config.HTTPConfig{MaxFileBytes: int64(len(jpegImage)), MaxRequestBytes: 8192, MaxImagePixels: 16}

	tests := []struct {
		name            string
		file            []byte
		secondFileField string
		padding         int
		trailing        int
		malformed       bool
		serviceErr      error
		wantStatus      int
		wantUploads     int
	}{
		{name: "JPEG at file and pixel limits", file: jpegImage, wantStatus: http.StatusAccepted, wantUploads: 1},
		{name: "PNG with JPEG metadata", file: pngImage, wantStatus: http.StatusAccepted, wantUploads: 1},
		{name: "file one byte too large", file: append(bytes.Clone(jpegImage), 0), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "body too large with valid file", file: pngImage, padding: 8192, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "trailing bytes exceed body limit", file: pngImage, trailing: 8192, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "malformed multipart", file: pngImage, malformed: true, wantStatus: http.StatusBadRequest},
		{name: "missing image field", wantStatus: http.StatusBadRequest},
		{name: "empty file", file: []byte{}, wantStatus: http.StatusBadRequest},
		{name: "two files in image field", file: pngImage, secondFileField: "image", wantStatus: http.StatusBadRequest},
		{name: "extra file field", file: pngImage, secondFileField: "extra", wantStatus: http.StatusBadRequest},
		{name: "text with JPEG metadata", file: []byte("not an image"), wantStatus: http.StatusBadRequest},
		{name: "truncated PNG", file: pngImage[:len(pngImage)-8], wantStatus: http.StatusBadRequest},
		{name: "unsupported GIF", file: gifImage, wantStatus: http.StatusBadRequest},
		{name: "image exceeds pixel limit", file: largePNG, wantStatus: http.StatusBadRequest},
		{name: "service failure", file: pngImage, serviceErr: errors.New("upload failed"), wantStatus: http.StatusInternalServerError, wantUploads: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			if tt.file != nil {
				addImagePart(t, form, "image", tt.file)
			}
			if tt.secondFileField != "" {
				addImagePart(t, form, tt.secondFileField, pngImage)
			}
			if err := form.WriteField("padding", strings.Repeat("x", tt.padding)); err != nil {
				t.Fatal(err)
			}
			if err := form.Close(); err != nil {
				t.Fatal(err)
			}
			body.WriteString(strings.Repeat("x", tt.trailing))

			request := httptest.NewRequest(http.MethodPost, "/api/analyze", &body)
			contentType := form.FormDataContentType()
			if tt.malformed {
				contentType = "multipart/form-data"
			}
			request.Header.Set("Content-Type", contentType)
			request.ContentLength = -1
			t.Cleanup(func() {
				if request.MultipartForm != nil {
					if err := request.MultipartForm.RemoveAll(); err != nil {
						t.Error(err)
					}
				}
			})

			service := &recordingService{id: uuid.NewV7(), err: tt.serviceErr}
			handler, err := New(service, discardLogger{}, limits)
			if err != nil {
				t.Fatal(err)
			}
			response := httptest.NewRecorder()
			handler.Upload(response, request)

			if response.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, tt.wantStatus)
			}
			if service.uploads != tt.wantUploads {
				t.Errorf("Upload calls = %d, want %d", service.uploads, tt.wantUploads)
			}
			if tt.wantUploads == 1 && !bytes.Equal(service.data, tt.file) {
				t.Error("Upload did not receive the original image bytes")
			}
			if tt.wantStatus == http.StatusAccepted {
				if got := response.Result().Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
				var result struct {
					TaskID string `json:"task_id"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result.TaskID != service.id.String() {
					t.Errorf("task_id = %q, want %q", result.TaskID, service.id.String())
				}
			}
		})
	}
}

func encodeImage(t *testing.T, size int, encode func(io.Writer, image.Image) error) []byte {
	t.Helper()
	var data bytes.Buffer
	if err := encode(&data, image.NewGray(image.Rect(0, 0, size, size))); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func addImagePart(t *testing.T, form *multipart.Writer, field string, data []byte) {
	t.Helper()
	header := make(textproto.MIMEHeader)
	header.Set("Content-Type", "image/jpeg")
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{"name": field, "filename": "image.jpg"}))
	file, err := form.CreatePart(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
}

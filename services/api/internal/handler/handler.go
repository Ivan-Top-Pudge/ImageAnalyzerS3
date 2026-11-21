// Package handler обрабатывает HTTP-запросы на загрузку изображений для анализа.
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"
	"uuid"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/config"
)

// Handler обрабатывает HTTP-запросы на загрузку изображений.
type Handler struct {
	service service
	logger  logger
	limits  config.HTTPConfig
}

type service interface {
	Upload(context.Context, []byte) (uuid.UUID, error)
}

type logger interface {
	Log(msg string)
}

// New создаёт обработчик загрузки и проверяет лимиты файла и тела запроса.
func New(service service, log logger, httpConfig config.HTTPConfig) (*Handler, error) {
	if httpConfig.MaxFileBytes <= 0 || httpConfig.MaxRequestBytes <= httpConfig.MaxFileBytes {
		return nil, errors.New("http.max_file_bytes must be positive " +
			"and http.max_request_bytes must be greater than it")
	}
	if httpConfig.MaxImagePixels <= 0 {
		return nil, errors.New("http.max_image_pixels must be positive")
	}

	return &Handler{
		service: service,
		logger:  log,
		limits:  httpConfig,
	}, nil
}

// Upload проверяет изображение из POST /api/analyze и передаёт его сервису.
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	const op = "handler.Upload"
	r.Body = http.MaxBytesReader(w, r.Body, h.limits.MaxRequestBytes)

	err := r.ParseMultipartForm(h.limits.MaxFileBytes)
	if err == nil {
		// Дочитываем оставшиеся байты в Body
		_, err = io.Copy(io.Discard, r.Body)
	}
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			return
		}
		h.logger.Log(fmt.Sprintf("%s: failed to read multipart request: %s", op, err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	FormFile, FileHeader, err := r.FormFile("image")
	if err != nil {
		h.logger.Log(fmt.Sprintf("%s: failed to parse form file: %s", op, err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer func(FormFile multipart.File) {
		err := FormFile.Close()
		if err != nil {
			h.logger.Log(fmt.Sprintf("%s: failed to close form file: %s", op, err))
		}
	}(FormFile)

	if FileHeader.Size == 0 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if FileHeader.Size > h.limits.MaxFileBytes {
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		return
	}

	data, err := io.ReadAll(FormFile)
	if err != nil {
		h.logger.Log(fmt.Sprintf("%s: failed to read image: %s", op, err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if err := validateImage(data, h.limits.MaxImagePixels); err != nil {
		h.logger.Log(fmt.Sprintf("%s: invalid image: %s", op, err))
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	id, err := h.service.Upload(r.Context(), data)
	if err != nil {
		h.logger.Log(fmt.Sprintf("%s: failed to upload image: %s", op, err))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	err = json.NewEncoder(w).Encode(struct {
		TaskID uuid.UUID `json:"task_id"`
	}{TaskID: id})
	if err != nil {
		h.logger.Log(fmt.Sprintf("%s: failed to write response: %s", op, err))
	}
}

func validateImage(data []byte, maxPixels int64) error {
	metadata, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read image metadata: %w", err)
	}
	if format != "jpeg" && format != "png" {
		return fmt.Errorf("unsupported image format %q", format)
	}
	if metadata.Width <= 0 || metadata.Height <= 0 || int64(metadata.Width) > maxPixels/int64(metadata.Height) {
		return fmt.Errorf("image dimensions %dx%d exceed the limit of %d pixels", metadata.Width, metadata.Height, maxPixels)
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("decode image: %w", err)
	}
	return nil
}

package analysis

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/domain"
)

type Analysis struct {
	s3      s3
	storage storage
}

type s3 interface {
	PutObject(ctx context.Context, key string, data []byte) error
	GetObject(ctx context.Context, key string) ([]byte, error)
	DeleteObject(ctx context.Context, key string) error
}

type storage interface {
	CreateWithOutbox(ctx context.Context, imageID string) (uuid.UUID, error)
}

func New(s3 s3, st storage) *Analysis {
	return &Analysis{
		s3:      s3,
		storage: st,
	}
}

// Upload сохраняет изображение в S3 хранилище, затем сохраняет задачу на анализ изображения в БД с ключом объекта в S3.
func (a *Analysis) Upload(ctx context.Context, fileBytes []byte) (uuid.UUID, error) {
	const op = "S3Storage.Upload"

	imageID := uuid.NewV7()
	strImageID := imageID.String()

	if err := a.s3.PutObject(ctx, strImageID, fileBytes); err != nil {
		return uuid.Nil(), fmt.Errorf("%s: %w", op, err)
	}

	taskID, err := a.storage.CreateWithOutbox(ctx, strImageID)
	if err != nil {
		if errors.Is(err, domain.ErrTaskNotCommitted) {
			// TODO: fix hardcode timeout
			delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()

			if delErr := a.s3.DeleteObject(delCtx, strImageID); delErr != nil {
				return uuid.Nil(), fmt.Errorf("%s: %w", op, errors.Join(delErr, err))
			}
		}
		return uuid.Nil(), fmt.Errorf("%s: %w", op, err)
	}

	return taskID, nil
}

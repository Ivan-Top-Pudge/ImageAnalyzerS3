package analysis

import (
	"context"
	"fmt"
	"uuid"
)

type Analysis struct {
	s3      s3
	storage storage
}

type s3 interface {
	PutObject(ctx context.Context, key string, data []byte) error
	GetObject(ctx context.Context, key string) ([]byte, error)
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
		// TODO: delete s3 object if transaction commit failed
		// if errors.Is(err, ErrTaskNotCommitted)
		return uuid.Nil(), fmt.Errorf("%s: %w", op, err)
	}

	return taskID, nil
}

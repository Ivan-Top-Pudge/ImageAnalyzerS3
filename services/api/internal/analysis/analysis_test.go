package analysis

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"uuid"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/domain"
)

type storageMock struct {
	create func(context.Context, string) (uuid.UUID, error)
}

func (m storageMock) CreateWithOutbox(ctx context.Context, objectKey string) (uuid.UUID, error) {
	return m.create(ctx, objectKey)
}

type s3Mock struct {
	put          func(context.Context, string, []byte) error
	deleteObject func(context.Context, string) error
}

func (m s3Mock) PutObject(ctx context.Context, key string, data []byte) error {
	return m.put(ctx, key, data)
}

func (s3Mock) GetObject(context.Context, string) ([]byte, error) {
	panic("unexpected GetObject call")
}

func (m s3Mock) DeleteObject(ctx context.Context, key string) error {
	return m.deleteObject(ctx, key)
}

func TestUpload(t *testing.T) {
	s3Err := errors.New("s3 unavailable")
	notCommittedErr := errors.Join(domain.ErrTaskNotCommitted, errors.New("outbox insert failed"))
	deleteErr := errors.New("delete object failed")

	tests := []struct {
		name          string
		putErr        error
		createErr     error
		deleteErr     error
		cancelRequest bool
		wantErr       error
		wantCalls     []string
		wantTaskID    uuid.UUID
	}{
		{
			name:       "returns the repository task ID after saving the image",
			wantCalls:  []string{"PutObject", "CreateWithOutbox"},
			wantTaskID: uuid.NewV7(),
		},
		{
			name:      "skips repository when S3 fails",
			putErr:    s3Err,
			wantErr:   s3Err,
			wantCalls: []string{"PutObject"},
		},
		{
			name:      "keeps the image when the commit outcome is unknown",
			createErr: context.DeadlineExceeded,
			wantErr:   context.DeadlineExceeded,
			wantCalls: []string{"PutObject", "CreateWithOutbox"},
		},
		{
			name:          "deletes the original object after a confirmed rollback even if the request is canceled",
			createErr:     notCommittedErr,
			cancelRequest: true,
			wantErr:       notCommittedErr,
			wantCalls:     []string{"PutObject", "CreateWithOutbox", "DeleteObject"},
		},
		{
			name:      "preserves the repository and deletion errors",
			createErr: notCommittedErr,
			deleteErr: deleteErr,
			wantErr:   notCommittedErr,
			wantCalls: []string{"PutObject", "CreateWithOutbox", "DeleteObject"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			image := []byte("test image")
			var calls []string
			var objectKey string

			s3 := s3Mock{
				put: func(_ context.Context, key string, data []byte) error {
					calls = append(calls, "PutObject")
					objectKey = key
					if key == "" || key == uuid.Nil().String() {
						t.Errorf("S3 received invalid object key %q", key)
					}
					if !bytes.Equal(data, image) {
						t.Errorf("S3 data = %q, want %q", data, image)
					}
					return tt.putErr
				},
				deleteObject: func(ctx context.Context, key string) error {
					calls = append(calls, "DeleteObject")
					if key != objectKey {
						t.Errorf("deleted object key = %q, want original S3 key %q", key, objectKey)
					}
					if err := ctx.Err(); err != nil {
						t.Errorf("DeleteObject received a canceled context: %v", err)
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Error("DeleteObject received a context without a deadline")
					}
					return tt.deleteErr
				},
			}
			repository := storageMock{
				create: func(_ context.Context, key string) (uuid.UUID, error) {
					calls = append(calls, "CreateWithOutbox")
					if key != objectKey {
						t.Errorf("repository object key = %q, want S3 key %q", key, objectKey)
					}
					if tt.cancelRequest {
						cancel()
					}
					return tt.wantTaskID, tt.createErr
				},
			}

			id, err := New(s3, repository).Upload(requestCtx, image)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Upload error = %v, want %v", err, tt.wantErr)
			}
			if tt.deleteErr != nil && !errors.Is(err, tt.deleteErr) {
				t.Errorf("Upload error = %v, want it to include %v", err, tt.deleteErr)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", calls, tt.wantCalls)
			}

			if id != tt.wantTaskID {
				t.Errorf("returned task ID = %s, want %s", id, tt.wantTaskID)
			}
		})
	}
}

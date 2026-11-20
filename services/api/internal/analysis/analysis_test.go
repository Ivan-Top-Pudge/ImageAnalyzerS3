package analysis

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"uuid"
)

type storageMock struct {
	create func(context.Context, string) error
}

func (m storageMock) CreateWithOutbox(ctx context.Context, taskID string) error {
	return m.create(ctx, taskID)
}

type s3Mock struct {
	put func(context.Context, string, []byte) error
}

func (m s3Mock) PutObject(ctx context.Context, key string, data []byte) error {
	return m.put(ctx, key, data)
}

func (s3Mock) GetObject(context.Context, string) ([]byte, error) {
	panic("unexpected GetObject call")
}

func TestUpload(t *testing.T) {
	s3Err := errors.New("S3 unavailable")
	repositoryErr := errors.New("database unavailable")

	tests := []struct {
		name      string
		putErr    error
		createErr error
		wantErr   error
		wantCalls []string
	}{
		{
			name:      "returns task ID after saving image and task",
			wantCalls: []string{"PutObject", "CreateWithOutbox"},
		},
		{
			name:      "skips repository when S3 fails",
			putErr:    s3Err,
			wantErr:   s3Err,
			wantCalls: []string{"PutObject"},
		},
		{
			name:      "returns repository error",
			createErr: repositoryErr,
			wantErr:   repositoryErr,
			wantCalls: []string{"PutObject", "CreateWithOutbox"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image := []byte("test image")
			var calls []string
			var objectKey string

			s3 := s3Mock{
				put: func(_ context.Context, key string, data []byte) error {
					calls = append(calls, "PutObject")
					objectKey = key
					if !bytes.Equal(data, image) {
						t.Errorf("S3 data = %q, want %q", data, image)
					}
					return tt.putErr
				},
			}
			repository := storageMock{
				create: func(_ context.Context, taskID string) error {
					calls = append(calls, "CreateWithOutbox")
					if taskID != objectKey {
						t.Errorf("repository task ID = %q, S3 key = %q", taskID, objectKey)
					}
					return tt.createErr
				},
			}

			id, err := New(s3, repository).Upload(context.Background(), image)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Upload error = %v, want %v", err, tt.wantErr)
			}
			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf("calls = %v, want %v", calls, tt.wantCalls)
			}

			if tt.wantErr != nil {
				if id != uuid.Nil() {
					t.Errorf("Upload returned task ID %s on error", id)
				}
				return
			}

			if id == uuid.Nil() || id.String() != objectKey {
				t.Errorf("returned task ID = %s, want non-nil ID matching S3 key %q", id, objectKey)
			}
		})
	}
}

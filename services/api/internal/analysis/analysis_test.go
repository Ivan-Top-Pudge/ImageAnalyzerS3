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
	create func(context.Context, string) (uuid.UUID, error)
}

func (m storageMock) CreateWithOutbox(ctx context.Context, objectKey string) (uuid.UUID, error) {
	return m.create(ctx, objectKey)
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
	s3Err := errors.New("s3 unavailable")
	repositoryErr := errors.New("database unavailable")

	tests := []struct {
		name       string
		putErr     error
		createErr  error
		wantErr    error
		wantCalls  []string
		wantTaskID uuid.UUID
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
					if key == "" || key == uuid.Nil().String() {
						t.Errorf("S3 received invalid object key %q", key)
					}
					if !bytes.Equal(data, image) {
						t.Errorf("S3 data = %q, want %q", data, image)
					}
					return tt.putErr
				},
			}
			repository := storageMock{
				create: func(_ context.Context, key string) (uuid.UUID, error) {
					calls = append(calls, "CreateWithOutbox")
					if key != objectKey {
						t.Errorf("repository object key = %q, want S3 key %q", key, objectKey)
					}
					return tt.wantTaskID, tt.createErr
				},
			}

			id, err := New(s3, repository).Upload(context.Background(), image)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Upload error = %v, want %v", err, tt.wantErr)
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

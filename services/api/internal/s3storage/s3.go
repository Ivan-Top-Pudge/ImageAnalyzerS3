package s3storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3 struct {
	client *s3.Client
	bucket string
}

func New(cfg aws.Config, baseEndpoint, region, bucket string, usePathStyle bool) *S3 {
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.Region = region
		o.UsePathStyle = usePathStyle
		o.BaseEndpoint = aws.String(baseEndpoint)
	})
	return &S3{client: client, bucket: bucket}
}

func (s *S3) PutObject(ctx context.Context, objectKey string, data []byte) error {
	const op = "S3Storage.PutObject"

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
		Body:   bytes.NewReader(data),
	})
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

func (s *S3) GetObject(ctx context.Context, objectKey string) ([]byte, error) {
	const op = "S3Storage.GetObject"

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			// Залогировать + метрика
		}
	}(output.Body)

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return data, nil
}

func (s *S3) ListObjects(ctx context.Context) {
	output, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
	})
	if err != nil {
		log.Fatal(err)
	}

	log.Println("first page results")
	for _, object := range output.Contents {
		log.Printf("key=%s size=%d", aws.ToString(object.Key), *object.Size)
	}
}

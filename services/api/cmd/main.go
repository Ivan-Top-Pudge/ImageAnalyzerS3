package main

import (
	"context"
	"log"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/config"
	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/s3storage"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
)

func main() {
	cfg := config.MustLoad()

	// Load the Shared AWS Configuration (~/.aws/config)
	awsCFG, err := awsconfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	// Create an Amazon S3 service client
	garage := cfg.GarageCFG
	storage := s3storage.New(awsCFG, garage.BaseEndpoint, garage.Region, garage.Bucket, garage.UsePathStyle)

	storage.ListObjects(context.TODO())
}

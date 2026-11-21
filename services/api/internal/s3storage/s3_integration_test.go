//go:build integration

package s3storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestDeleteObjectGarage(t *testing.T) {
	const (
		port      = "3900/tcp"
		region    = "garage"
		bucket    = "default-bucket"
		objectKey = "image-to-delete"
	)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	accessKey := "GK" + randomHex(t, 16)
	secretKey := randomHex(t, 32)
	config := fmt.Sprintf(`metadata_dir = "/tmp/meta"
data_dir = "/tmp/data"
db_engine = "sqlite"
replication_factor = 1
rpc_bind_addr = "[::]:3901"
rpc_public_addr = "127.0.0.1:3901"
rpc_secret = "%s"

[s3_api]
s3_region = "%s"
api_bind_addr = "[::]:3900"
root_domain = ".s3.garage.localhost"
`, randomHex(t, 32), region)

	garage, err := testcontainers.Run(ctx, "dxflrs/garage:v2.5.0",
		testcontainers.WithCmd("/garage", "server", "--single-node", "--default-bucket"),
		testcontainers.WithEnv(map[string]string{
			"GARAGE_DEFAULT_ACCESS_KEY": accessKey,
			"GARAGE_DEFAULT_SECRET_KEY": secretKey,
			"GARAGE_DEFAULT_BUCKET":     bucket,
		}),
		testcontainers.WithFiles(testcontainers.ContainerFile{
			Reader:            strings.NewReader(config),
			ContainerFilePath: "/etc/garage.toml",
			FileMode:          0o600,
		}),
		testcontainers.WithExposedPorts(port),
		testcontainers.WithHostConfigModifier(func(cfg *container.HostConfig) {
			cfg.PortBindings = network.PortMap{
				network.MustParsePort(port): {{HostIP: netip.MustParseAddr("127.0.0.1")}},
			}
		}),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/").WithPort(port).
			WithStatusCodeMatcher(func(status int) bool { return status == http.StatusForbidden })),
	)
	testcontainers.CleanupContainer(t, garage)
	if err != nil {
		t.Fatalf("start Garage: %v", err)
	}

	t.Log("garage container started")

	endpoint, err := garage.PortEndpoint(ctx, port, "http")
	if err != nil {
		t.Fatalf("get Garage endpoint: %v", err)
	}

	storage := New(aws.Config{
		Credentials: credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	}, endpoint, region, bucket, true)
	content := []byte("Garage integration test object")
	if err := storage.PutObject(ctx, objectKey, content); err != nil {
		t.Fatalf("put object: %v", err)
	}
	stored, err := storage.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatalf("get object before deletion: %v", err)
	}
	if !bytes.Equal(stored, content) {
		t.Fatalf("object body = %q, want %q", stored, content)
	}

	if err := storage.DeleteObject(ctx, objectKey); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if _, err := storage.GetObject(ctx, objectKey); err == nil {
		t.Fatal("deleted object is still readable")
	} else if _, ok := errors.AsType[*types.NoSuchKey](err); !ok {
		t.Fatalf("get deleted object: expected NoSuchKey, got %v", err)
	}
}

func randomHex(t *testing.T, size int) string {
	t.Helper()
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		t.Fatalf("generate test secret: %v", err)
	}
	return hex.EncodeToString(value)
}

// Package app собирает зависимости API и управляет их завершением.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/analysis"
	httpserver "github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/app/http"
	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/config"
	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/handler"
	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/postgres"
	"github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/s3storage"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	startupTimeout     = 10 * time.Second
	defaultHTTPAddress = ":8080"
)

// App содержит HTTP-сервер и владеет пулом соединений PostgreSQL.
type App struct {
	HTTPSrv *httpserver.Server
	pool    *pgxpool.Pool
}

// New загружает конфигурацию и собирает приложение. При ошибке инициализации вызывает панику.
func New(ctx context.Context) *App {
	cfg := config.MustLoad()
	logger := setupLogger(cfg.Env)
	startupCtx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	imageStorage := mustSetupS3(startupCtx, cfg.GarageCFG)
	pool := mustSetupPostgres(startupCtx, cfg.PostgresDSN)
	ready := false
	defer func() {
		if !ready {
			pool.Close()
		}
	}()

	service := analysis.New(imageStorage, postgres.New(pool))
	routes := mustSetupRoutes(service, logger, cfg.HTTP)
	server := httpserver.New(httpAddress(), routes, logger)

	ready = true
	return &App{HTTPSrv: server, pool: pool}
}

// Shutdown ждёт завершения активных HTTP-запросов и закрывает пул PostgreSQL.
func (a *App) Shutdown(ctx context.Context) error {
	if err := a.HTTPSrv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown HTTP server: %w", err)
	}
	a.pool.Close()
	return nil
}

func setupLogger(env string) *slog.Logger {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil)).With("env", env)
	slog.SetDefault(logger)
	return logger
}

func mustSetupS3(ctx context.Context, cfg config.GarageConfig) *s3storage.S3 {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		panic(fmt.Errorf("load AWS configuration: %w", err))
	}

	s3 := s3storage.New(awsCfg, cfg.BaseEndpoint, cfg.Region, cfg.Bucket, cfg.UsePathStyle)

	if err := s3.Ping(ctx); err != nil {
		panic(fmt.Errorf("ping S3: %w", err))
	}
	return s3
}

func mustSetupPostgres(ctx context.Context, dsn string) *pgxpool.Pool {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		panic(fmt.Errorf("initialize PostgreSQL pool: %w", err))
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		panic(fmt.Errorf("connect to PostgreSQL: %w", err))
	}
	return pool
}

func mustSetupRoutes(service *analysis.Analysis, logger *slog.Logger, cfg config.HTTPConfig) *http.ServeMux {
	upload, err := handler.New(service, handlerLogger{logger: logger}, cfg)
	if err != nil {
		panic(fmt.Errorf("configure upload handler: %w", err))
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/analyze", upload.Upload)
	return mux
}

func httpAddress() string {
	if address := os.Getenv("HTTP_ADDR"); address != "" {
		return address
	}
	return defaultHTTPAddress
}

type handlerLogger struct {
	logger *slog.Logger
}

func (l handlerLogger) Log(msg string) {
	l.logger.Error(msg)
}

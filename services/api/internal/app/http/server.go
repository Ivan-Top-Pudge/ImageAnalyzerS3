// Package httpserver запускает и останавливает HTTP-сервер API.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const readHeaderTimeout = 5 * time.Second

// Server управляет запуском и завершением HTTP-сервера.
type Server struct {
	server *http.Server
	logger *slog.Logger
}

// New создаёт HTTP-сервер с переданным маршрутизатором и логгером приложения.
func New(address string, routes http.Handler, logger *slog.Logger) *Server {
	return &Server{
		server: &http.Server{
			Addr:              address,
			Handler:           routes,
			ReadHeaderTimeout: readHeaderTimeout,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
		},
		logger: logger,
	}
}

// MustRun принимает HTTP-запросы до завершения сервера. При ошибке запуска вызывает панику.
func (s *Server) MustRun() {
	s.logger.Info("starting HTTP server", "addr", s.server.Addr)
	if err := s.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		panic(fmt.Errorf("run HTTP server: %w", err))
	}
}

// Shutdown прекращает приём запросов и ждёт завершения активных обработчиков.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

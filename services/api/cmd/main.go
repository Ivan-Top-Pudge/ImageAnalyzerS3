package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	apiapp "github.com/Ivan-Top-Pudge/ImageAnalyzerS3/services/api/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := apiapp.New(ctx)
	go app.HTTPSrv.MustRun()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.Shutdown(shutdownCtx); err != nil {
		panic(err)
	}
}

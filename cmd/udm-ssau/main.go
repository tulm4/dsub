// Command udm-ssau is the entry point for the Nudm_SSAU microservice.
//
// Based on: docs/service-decomposition.md §2.7 (udm-ssau)
// 3GPP: TS 29.503 Nudm_SSAU — Service-Specific Authorization service
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tulm4/dsub/internal/common/config"
	"github.com/tulm4/dsub/internal/ssau"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("udm-ssau: load config: %v", err)
	}
	cfg.ServiceName = "udm-ssau"

	if cfg.DBDSN == "" {
		log.Fatal("udm-ssau: UDM_DB_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("udm-ssau: create DB pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("udm-ssau: ping DB: %v", err)
	}

	svc := ssau.NewService(pool)
	handler := ssau.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("udm-ssau: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("udm-ssau: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("udm-ssau: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("udm-ssau: shutdown: %v", err)
	}
	log.Println("udm-ssau: stopped")
}

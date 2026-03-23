// Command udm-pp is the entry point for the Nudm_PP microservice.
//
// Based on: docs/service-decomposition.md §2.5 (udm-pp)
// 3GPP: TS 29.503 Nudm_PP — Parameter Provisioning service
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
	"github.com/tulm4/dsub/internal/pp"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("udm-pp: load config: %v", err)
	}
	cfg.ServiceName = "udm-pp"

	if cfg.DBDSN == "" {
		log.Fatal("udm-pp: UDM_DB_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("udm-pp: create DB pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("udm-pp: ping DB: %v", err)
	}

	svc := pp.NewService(pool)
	handler := pp.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("udm-pp: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("udm-pp: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("udm-pp: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("udm-pp: shutdown: %v", err)
	}
	log.Println("udm-pp: stopped")
}

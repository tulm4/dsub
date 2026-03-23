// Command udm-rsds is the entry point for the Nudm_RSDS microservice.
//
// Based on: docs/service-decomposition.md §2.9 (udm-rsds)
// 3GPP: TS 29.503 Nudm_RSDS — Report SMS Delivery Status service
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
	"github.com/tulm4/dsub/internal/rsds"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("udm-rsds: load config: %v", err)
	}
	cfg.ServiceName = "udm-rsds"

	if cfg.DBDSN == "" {
		log.Fatal("udm-rsds: UDM_DB_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("udm-rsds: create DB pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("udm-rsds: ping DB: %v", err)
	}

	svc := rsds.NewService(pool)
	handler := rsds.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("udm-rsds: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("udm-rsds: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("udm-rsds: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("udm-rsds: shutdown: %v", err)
	}
	log.Println("udm-rsds: stopped")
}

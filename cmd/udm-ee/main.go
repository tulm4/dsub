// Command udm-ee is the entry point for the Nudm_EE microservice.
//
// Based on: docs/service-decomposition.md §2.4 (udm-ee)
// 3GPP: TS 29.503 Nudm_EE — Event Exposure service
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
	"github.com/tulm4/dsub/internal/ee"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("udm-ee: load config: %v", err)
	}
	cfg.ServiceName = "udm-ee"

	if cfg.DBDSN == "" {
		log.Fatal("udm-ee: UDM_DB_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("udm-ee: create DB pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("udm-ee: ping DB: %v", err)
	}

	svc := ee.NewService(pool)
	handler := ee.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("udm-ee: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("udm-ee: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("udm-ee: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("udm-ee: shutdown: %v", err)
	}
	log.Println("udm-ee: stopped")
}

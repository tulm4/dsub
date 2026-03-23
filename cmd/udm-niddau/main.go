// Command udm-niddau is the entry point for the Nudm_NIDDAU microservice.
//
// Based on: docs/service-decomposition.md §2.8 (udm-niddau)
// 3GPP: TS 29.503 Nudm_NIDDAU — NIDD Authorization service
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
	"github.com/tulm4/dsub/internal/niddau"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("udm-niddau: load config: %v", err)
	}
	cfg.ServiceName = "udm-niddau"

	if cfg.DBDSN == "" {
		log.Fatal("udm-niddau: UDM_DB_DSN is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DBDSN)
	if err != nil {
		log.Fatalf("udm-niddau: create DB pool: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("udm-niddau: ping DB: %v", err)
	}

	svc := niddau.NewService(pool)
	handler := niddau.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	addr := fmt.Sprintf(":%d", cfg.HTTPPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		log.Printf("udm-niddau: listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("udm-niddau: listen: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("udm-niddau: shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("udm-niddau: shutdown: %v", err)
	}
	log.Println("udm-niddau: stopped")
}

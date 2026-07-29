package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/config"
	"github.com/nempay/api/internal/devseed"
	"github.com/nempay/api/internal/httpapi"
)

// bankTimeout bounds each bank-sim call; tok_timeout sleeps past it so the api sees a timeout.
const bankTimeout = 2 * time.Second

func main() {
	cfg := config.Load()

	// One pgx pool for the process; the router and services share it. A ping up front turns a
	// bad DB_URL / down database into a clear boot failure instead of a first-request 500.
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()
	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	if err := pool.Ping(pingCtx); err != nil {
		cancelPing()
		log.Fatalf("db unreachable: %v", err)
	}
	cancelPing()

	if cfg.DevSeed {
		if err := devseed.Run(ctx, pool); err != nil {
			log.Fatalf("dev seed: %v", err)
		}
	}

	bank := banksim.New(cfg.BankSimURL, bankTimeout)

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httpapi.NewRouter(pool, bank),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("nempay api listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown: finish in-flight requests before exiting.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down api...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	log.Println("api stopped")
}

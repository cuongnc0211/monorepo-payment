// Command worker runs NemPay's out-of-band background work — the notification plane plus the
// periodic money-plane sweeps — as a process SEPARATE from the api. Modelling delivery as a real
// durable queue (asynq over Redis) rather than inline HTTP from the request is a core lesson: a
// merchant outage must never block or roll back a money movement.
//
// Responsibilities:
//   - asynq server: processes "webhook:deliver" tasks by delivering one outbox row.
//   - dispatcher: periodically claims due outbox rows and enqueues a delivery task for each.
//   - sweeps: settle captured intents (async batched settlement), expire stale intents, and
//     free/expire idempotency keys.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/nempay/api/internal/banksim"
	"github.com/nempay/api/internal/config"
	"github.com/nempay/api/internal/repository/db"
	"github.com/nempay/api/internal/service"
	"github.com/nempay/api/internal/webhook"
)

const (
	taskDeliverWebhook = "webhook:deliver"

	dispatchInterval = 1 * time.Second
	dispatchBatch    = 100

	settleInterval = 5 * time.Second
	settleDelay    = 10 * time.Second // compressed "T+1": captured intents settle after this

	expireInterval = 30 * time.Second
	expireDelay    = 15 * time.Minute

	idemSweepInterval = 60 * time.Second
	idemOrphanAfter   = 5 * time.Minute
	idemCompletedTTL  = 24 * time.Hour

	bankTimeout = 2 * time.Second
)

type deliverPayload struct {
	OutboxID uuid.UUID `json:"outbox_id"`
}

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("db unreachable: %v", err)
	}

	redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		log.Fatalf("redis url: %v", err)
	}
	client := asynq.NewClient(redisOpt)
	defer client.Close()

	processor := webhook.NewProcessor(pool)
	intents := service.NewIntents(pool, banksim.New(cfg.BankSimURL, bankTimeout))
	q := db.New(pool)

	// asynq server: deliver one outbox row per task. The task handler ALWAYS returns nil — the DB
	// outbox is the single retry authority (deliverRow records success/retry/dead there), so asynq
	// must complete the task (and free its TaskID) rather than archive-on-error and block re-
	// dispatch. Internal errors are logged; the row stays pending and is re-dispatched after its
	// backoff.
	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 10})
	mux := asynq.NewServeMux()
	mux.HandleFunc(taskDeliverWebhook, func(ctx context.Context, t *asynq.Task) error {
		var p deliverPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			log.Printf("deliver: bad task payload: %v", err)
			return nil
		}
		if err := processor.DeliverByID(ctx, p.OutboxID); err != nil {
			log.Printf("deliver %s: %v", p.OutboxID, err)
		}
		return nil
	})
	if err := srv.Start(mux); err != nil { // non-blocking; shutdown driven by ctx below
		log.Fatalf("asynq server: %v", err)
	}

	// Dispatcher: claim due outbox rows and enqueue a delivery task for each. TaskID = row id makes
	// the enqueue idempotent: while a row's delivery task is pending/active, a concurrent dispatch
	// tick can't queue a second — so a single row is delivered once per due window, not N times.
	go every(ctx, dispatchInterval, "dispatch", func(ctx context.Context) (int, error) {
		return processor.Dispatch(ctx, dispatchBatch, func(id uuid.UUID) error {
			payload, _ := json.Marshal(deliverPayload{OutboxID: id})
			_, err := client.EnqueueContext(ctx, asynq.NewTask(taskDeliverWebhook, payload),
				asynq.MaxRetry(0), asynq.TaskID(id.String()))
			if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
				return nil // already queued/active for this row — nothing to do
			}
			return err
		})
	})

	// Money-plane sweeps.
	go every(ctx, settleInterval, "settle", func(ctx context.Context) (int, error) {
		return intents.SettleDueIntents(ctx, settleDelay)
	})
	go every(ctx, expireInterval, "expire", func(ctx context.Context) (int, error) {
		return intents.ExpireStaleIntents(ctx, expireDelay)
	})

	// Idempotency housekeeping (from task-03): free orphaned in-flight rows and expire completed.
	go every(ctx, idemSweepInterval, "idem-sweep", func(ctx context.Context) (int, error) {
		if err := service.SweepOrphanedKeys(ctx, q, idemOrphanAfter); err != nil {
			return 0, err
		}
		return 0, service.ExpireCompletedKeys(ctx, q, idemCompletedTTL)
	})

	log.Println("nempay worker started")
	<-ctx.Done()
	log.Println("shutting down worker...")
	srv.Shutdown()
	log.Println("worker stopped")
}

// every runs fn on an interval until ctx is done, logging (but not dying on) errors so one bad
// tick never kills the loop. A non-zero count is logged as progress.
func every(ctx context.Context, interval time.Duration, name string, fn func(context.Context) (int, error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := fn(ctx)
			if err != nil {
				log.Printf("%s: %v", name, err)
			} else if n > 0 {
				log.Printf("%s: processed %d", name, n)
			}
		}
	}
}

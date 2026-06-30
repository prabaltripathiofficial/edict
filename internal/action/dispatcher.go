package action

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// RetryPolicy controls at-least-once delivery. Each action attempt that fails
// is retried up to MaxAttempts total, sleeping with exponential backoff
// (BaseDelay * 2^(attempt-1)) capped at MaxDelay between tries.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// DefaultRetryPolicy is a sensible starting point: three tries, 200ms base.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond, MaxDelay: 5 * time.Second}
}

// AuditRecord captures one action attempt for the audit trail. Every fire is
// recorded — successes and failures — fulfilling Edict's audit-first goal.
type AuditRecord struct {
	RuleID     string
	ActionType string
	Attempt    int
	Success    bool
	Err        error
	Duration   time.Duration
	Event      event.ChangeEvent
}

// Auditor receives a record for every action attempt. The default implementation
// logs structured records; production deployments can persist them instead.
type Auditor interface {
	Record(ctx context.Context, rec AuditRecord)
}

// slogAuditor is the default Auditor.
type slogAuditor struct{ log *slog.Logger }

func (a slogAuditor) Record(ctx context.Context, rec AuditRecord) {
	a.log.LogAttrs(ctx, slog.LevelInfo, "edict.audit",
		slog.String("rule", rec.RuleID),
		slog.String("action", rec.ActionType),
		slog.Int("attempt", rec.Attempt),
		slog.Bool("success", rec.Success),
		slog.Duration("duration", rec.Duration),
		slog.Any("err", rec.Err),
		slog.Any("documentKey", rec.Event.DocumentKey),
	)
}

// Dispatcher builds each rule's actions once and runs them with retry on every
// fire. It implements engine.Dispatcher.
type Dispatcher struct {
	byRule  map[string][]Action
	policy  RetryPolicy
	auditor Auditor
}

// NewDispatcher precompiles the actions for every rule in the set, surfacing
// any malformed action spec (bad URL, unknown type) as a build error before the
// engine ever runs. A nil auditor falls back to structured logging.
func NewDispatcher(set *rule.Set, policy RetryPolicy, auditor Auditor) (*Dispatcher, error) {
	if auditor == nil {
		auditor = slogAuditor{log: slog.Default()}
	}
	byRule := make(map[string][]Action)
	for _, r := range set.All() {
		actions := make([]Action, 0, len(r.Actions))
		for i, spec := range r.Actions {
			a, err := Build(spec)
			if err != nil {
				return nil, fmt.Errorf("rule %q action %d: %w", r.ID, i, err)
			}
			actions = append(actions, a)
		}
		byRule[r.ID] = actions
	}
	return &Dispatcher{byRule: byRule, policy: policy, auditor: auditor}, nil
}

// Dispatch runs every action of a fired rule in order. Each action gets its own
// retry budget; if one action exhausts its retries the error is collected and
// the remaining actions still run, so an unreachable webhook does not suppress a
// sibling log/queue action.
func (d *Dispatcher) Dispatch(ctx context.Context, r rule.Rule, ev event.ChangeEvent) error {
	var errs []error
	for _, a := range d.byRule[r.ID] {
		if err := d.runWithRetry(ctx, a, ev, r); err != nil {
			errs = append(errs, fmt.Errorf("action %s: %w", a.Type(), err))
		}
	}
	return errors.Join(errs...)
}

// runWithRetry executes a single action, retrying with exponential backoff until
// it succeeds or the attempt budget is exhausted. Context cancellation aborts
// immediately and is returned as-is.
func (d *Dispatcher) runWithRetry(ctx context.Context, a Action, ev event.ChangeEvent, r rule.Rule) error {
	attempts := d.policy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		start := time.Now()
		err := a.Execute(ctx, ev, r)
		d.auditor.Record(ctx, AuditRecord{
			RuleID:     r.ID,
			ActionType: a.Type(),
			Attempt:    attempt,
			Success:    err == nil,
			Err:        err,
			Duration:   time.Since(start),
			Event:      ev,
		})
		if err == nil {
			return nil
		}
		lastErr = err

		if attempt == attempts {
			break
		}
		if err := sleepBackoff(ctx, d.policy, attempt); err != nil {
			return err
		}
	}
	return fmt.Errorf("exhausted %d attempts: %w", attempts, lastErr)
}

// sleepBackoff waits BaseDelay*2^(attempt-1), capped at MaxDelay, returning
// early if the context is cancelled.
func sleepBackoff(ctx context.Context, p RetryPolicy, attempt int) error {
	delay := p.BaseDelay << (attempt - 1)
	if p.MaxDelay > 0 && delay > p.MaxDelay {
		delay = p.MaxDelay
	}
	t := time.NewTimer(delay)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

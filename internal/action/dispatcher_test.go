package action

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

// discardAuditor swallows audit records so retry tests stay quiet.
type discardAuditor struct{}

func (discardAuditor) Record(context.Context, AuditRecord) {}

func slogAuditorDiscard() Auditor { return discardAuditor{} }

// flakyAction fails failures times, then succeeds, counting calls.
type flakyAction struct {
	failures int32
	calls    int32
}

func (f *flakyAction) Type() string { return "flaky" }

func (f *flakyAction) Execute(_ context.Context, _ event.ChangeEvent, _ rule.Rule) error {
	n := atomic.AddInt32(&f.calls, 1)
	if n <= f.failures {
		return errContext
	}
	return nil
}

// errContext is a sentinel non-context error used by flakyAction.
var errContext = &retryableErr{}

type retryableErr struct{}

func (*retryableErr) Error() string { return "transient failure" }

// fastPolicy keeps retry tests quick.
func fastPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
}

func TestRunWithRetry_SucceedsAfterFailures(t *testing.T) {
	d := &Dispatcher{policy: fastPolicy(), auditor: slogAuditorDiscard()}
	a := &flakyAction{failures: 2}

	err := d.runWithRetry(context.Background(), a, event.ChangeEvent{}, rule.Rule{ID: "r"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if got := atomic.LoadInt32(&a.calls); got != 3 {
		t.Fatalf("called %d times, want 3", got)
	}
}

func TestRunWithRetry_ExhaustsAttempts(t *testing.T) {
	d := &Dispatcher{policy: fastPolicy(), auditor: slogAuditorDiscard()}
	a := &flakyAction{failures: 99}

	if err := d.runWithRetry(context.Background(), a, event.ChangeEvent{}, rule.Rule{ID: "r"}); err == nil {
		t.Fatal("expected error after exhausting attempts")
	}
	if got := atomic.LoadInt32(&a.calls); got != 3 {
		t.Fatalf("called %d times, want 3 (MaxAttempts)", got)
	}
}

func TestRunWithRetry_ContextCancel(t *testing.T) {
	d := &Dispatcher{policy: RetryPolicy{MaxAttempts: 5, BaseDelay: time.Hour}, auditor: slogAuditorDiscard()}
	a := &flakyAction{failures: 99}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled: backoff sleep must return immediately

	if err := d.runWithRetry(ctx, a, event.ChangeEvent{}, rule.Rule{ID: "r"}); err == nil {
		t.Fatal("expected context error")
	}
	if got := atomic.LoadInt32(&a.calls); got != 1 {
		t.Fatalf("called %d times, want 1 before cancel aborted backoff", got)
	}
}

func TestWebhookAction_Delivers(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing json content type")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a, err := buildWebhook(rule.ActionSpec{Type: "webhook", URL: srv.URL})
	if err != nil {
		t.Fatalf("buildWebhook: %v", err)
	}
	if err := a.Execute(context.Background(), event.ChangeEvent{Collection: "orders"}, rule.Rule{ID: "r"}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("webhook hit %d times, want 1", hits)
	}
}

func TestWebhookAction_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	a, _ := buildWebhook(rule.ActionSpec{Type: "webhook", URL: srv.URL})
	if err := a.Execute(context.Background(), event.ChangeEvent{}, rule.Rule{ID: "r"}); err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestNewDispatcher_RejectsBadSpec(t *testing.T) {
	set := rule.NewSet([]rule.Rule{
		{ID: "r", Collection: "c", Actions: []rule.ActionSpec{{Type: "webhook"}}}, // missing URL
	})
	if _, err := NewDispatcher(set, DefaultRetryPolicy(), nil); err == nil {
		t.Fatal("expected error building dispatcher with invalid webhook spec")
	}
}

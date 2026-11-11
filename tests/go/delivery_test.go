package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	"github.com/google/uuid"

	"speechToText/src/consumer"
	"speechToText/src/db"
	"speechToText/src/types"
)

func TestDelivery(t *testing.T) {
	if os.Getenv("STT_INTEGRATION") != "1" {
		t.Skip("set STT_INTEGRATION=1 with PostgreSQL, Redis and RabbitMQ running")
	}
	if testStore == nil {
		t.Fatal("PostgreSQL must be available")
	}
	broker := os.Getenv("RABBITMQ_URL")
	if broker == "" {
		t.Fatal("RABBITMQ_URL is required")
	}

	owner := uuid.NewString()
	register := httptest.NewRecorder()
	loginBody := `{"username":"` + owner + `","password":"test-password"}`
	testHandlers.Register(register, httptest.NewRequest("POST", "/register", strings.NewReader(loginBody)))
	if register.Code != http.StatusOK {
		t.Fatalf("register: %d %s", register.Code, register.Body)
	}
	login := httptest.NewRecorder()
	testHandlers.Login(login, httptest.NewRequest("POST", "/login", strings.NewReader(loginBody)))
	if login.Code != http.StatusOK || len(login.Result().Cookies()) != 1 {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	cookie := login.Result().Cookies()[0]

	submit := func(key, audio string, wantCode int) string {
		t.Helper()
		body, err := json.Marshal(types.AudioRequest{Audio: audio})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/audio", strings.NewReader(string(body)))
		req.AddCookie(cookie)
		req.Header.Set("Idempotency-Key", key)
		rec := httptest.NewRecorder()
		testHandlers.Audio(rec, req)
		if rec.Code != wantCode {
			t.Fatalf("submit: got %d, want %d: %s", rec.Code, wantCode, rec.Body)
		}
		if wantCode != http.StatusAccepted {
			return ""
		}
		var response types.GetInfoResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil || response.Task_id == "" {
			t.Fatalf("invalid accepted response: %s", rec.Body)
		}
		t.Cleanup(func() { _ = testStore.DeleteTask(context.Background(), response.Task_id, owner) })
		return response.Task_id
	}

	key := uuid.NewString()
	id := submit(key, "https://example.com/retry.wav", http.StatusAccepted)
	if duplicate := submit(key, "https://example.com/retry.wav", http.StatusAccepted); duplicate != id {
		t.Fatalf("duplicate request created %s instead of %s", duplicate, id)
	}
	submit(key, "https://example.com/other.wav", http.StatusConflict)
	failed := submit(uuid.NewString(), "https://example.com/invalid.wav", http.StatusAccepted)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	status, _, err := testStore.ReadTask(ctx, id, owner)
	if err != nil || status != "queued" {
		t.Fatalf("before publisher start: %q, %v", status, err)
	}

	var calls, invalidCalls atomic.Int32
	publisherDone, workerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(publisherDone)
		consumer.RunPublisher(ctx, testStore, broker)
	}()
	go func() {
		defer close(workerDone)
		consumer.RunWorker(ctx, testStore, broker, func(bool) {}, nil, func(_ context.Context, audio string) (string, error) {
			switch audio {
			case "https://example.com/retry.wav":
				if calls.Add(1) == 1 {
					return "", &interfaces.StatusError{Resp: &http.Response{StatusCode: 429}}
				}
			case "https://example.com/invalid.wav":
				invalidCalls.Add(1)
				return "", &interfaces.StatusError{Resp: &http.Response{StatusCode: 400}}
			}
			return "test transcript", nil
		})
	}()
	defer func() {
		cancel()
		<-publisherDone
		<-workerDone
	}()

	awaitStatus := func(id, want string) *string {
		t.Helper()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			status, result, err := testStore.ReadTask(ctx, id, owner)
			if err != nil {
				t.Fatal(err)
			}
			if status == want {
				return result
			}
			select {
			case <-ctx.Done():
				t.Fatalf("task %s is %q, expected %q", id, status, want)
			case <-ticker.C:
			}
		}
	}

	result := awaitStatus(id, "completed")
	if result == nil || *result != "test transcript" || calls.Load() != 2 {
		t.Fatalf("unexpected result or attempt count: %v, %d", result, calls.Load())
	}
	if result := awaitStatus(failed, "failed"); result != nil || invalidCalls.Load() != 1 {
		t.Fatalf("permanent rejection was retried or returned a result")
	}
	_, err = testStore.ProcessTask(ctx, id, 2, func(string) (string, bool, error) {
		t.Error("duplicate completed delivery called the provider")
		return "", false, errors.New("duplicate")
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = testStore.ReadTask(ctx, id, "another-user")
	if err == nil {
		t.Fatal("read task as a different owner")
	}

	if os.Getenv("STT_LOAD") == "1" {
		const count = 60
		accepted := make([]time.Duration, 0, count)
		completed := make([]time.Duration, 0, count)
		started := make(map[string]time.Time, count)
		for range count {
			begin := time.Now()
			id := submit(uuid.NewString(), "https://example.com/load.wav", http.StatusAccepted)
			accepted = append(accepted, time.Since(begin))
			started[id] = begin
		}
		for len(started) > 0 {
			for id, begin := range started {
				status, _, err := testStore.ReadTask(ctx, id, owner)
				if err != nil {
					t.Fatal(err)
				}
				if status == "completed" {
					completed = append(completed, time.Since(begin))
					delete(started, id)
				}
			}
			select {
			case <-ctx.Done():
				t.Fatalf("load run left %d tasks pending", len(started))
			case <-time.After(10 * time.Millisecond):
			}
		}
		slices.Sort(accepted)
		slices.Sort(completed)
		index := (95*count+99)/100 - 1
		t.Logf("load: %d/%d completed, p95 accept %s, p95 observed completion %s",
			len(completed), count, accepted[index].Round(time.Microsecond), completed[index].Round(time.Millisecond))
	}

}

func TestExpiredLeaseRecovery(t *testing.T) {
	if os.Getenv("STT_INTEGRATION") != "1" {
		t.Skip("requires the integration database")
	}
	if testStore == nil {
		t.Fatal("PostgreSQL must be available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, owner := uuid.NewString(), uuid.NewString()
	if _, err := testStore.Enqueue(ctx, id, owner, "https://example.com/lease.wav", ""); err != nil {
		t.Fatal(err)
	}
	defer testStore.DeleteTask(context.Background(), id, owner)
	found, err := testStore.PublishNext(ctx, func(_ context.Context, published string, _ int) error {
		if published != id {
			return errors.New("expected an isolated database")
		}
		return nil
	})
	if err != nil || !found {
		t.Fatalf("publish lease test task: found=%v error=%v", found, err)
	}

	crashed, stop := context.WithCancel(ctx)
	defer stop()
	_, err = testStore.ProcessTask(crashed, id, 1, func(string) (string, bool, error) {
		stop()
		return "", true, context.Canceled
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	_, err = testStore.ProcessTask(ctx, id, 1, func(string) (string, bool, error) {
		t.Error("active lease allowed another worker")
		return "", false, nil
	})
	if !errors.Is(err, db.ErrDeliveryPending) {
		t.Fatalf("expected pending lease, got %v", err)
	}
	if _, err := testSQL.ExecContext(ctx, "UPDATE tasks SET lease_until = now() WHERE task_id = $1", id); err != nil {
		t.Fatal(err)
	}
	if _, err := testStore.ProcessTask(ctx, id, 1, func(string) (string, bool, error) {
		return "recovered", false, nil
	}); err != nil {
		t.Fatal(err)
	}
	status, result, err := testStore.ReadTask(ctx, id, owner)
	if err != nil || status != "completed" || result == nil || *result != "recovered" {
		t.Fatalf("failed to recover lease: %s %v %v", status, result, err)
	}
}

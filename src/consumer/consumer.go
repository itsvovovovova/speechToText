package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	amqp "github.com/rabbitmq/amqp091-go"

	"speechToText/src/db"
	"speechToText/src/metrics"
)

const queueName = "transcription.v2"

type delivery struct {
	TaskID  string `json:"task_id"`
	Attempt int    `json:"attempt"`
}

func connect(url string) (*amqp.Connection, error) {
	return amqp.DialConfig(url, amqp.Config{Dial: amqp.DefaultDial(5 * time.Second), Heartbeat: 5 * time.Second, Locale: "en_US"})
}

func declare(ch *amqp.Channel) error {
	if _, err := ch.QueueDeclare(queueName+".dead", true, false, false, false, nil); err != nil {
		return err
	}
	_, err := ch.QueueDeclare(queueName, true, false, false, false, amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": queueName + ".dead",
	})
	return err
}

func RunPublisher(ctx context.Context, store *db.Store, url string) {
	supervise(ctx, "publisher", url, func(ctx context.Context, ch *amqp.Channel) error {
		returned := ch.NotifyReturn(make(chan amqp.Return, 1))
		if err := ch.Confirm(false); err != nil {
			return err
		}
		for ctx.Err() == nil {
			attemptCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			found, err := store.PublishNext(attemptCtx, func(ctx context.Context, id string, attempt int) error {
				body, err := json.Marshal(delivery{TaskID: id, Attempt: attempt})
				if err != nil {
					return err
				}
				confirmation, err := ch.PublishWithDeferredConfirmWithContext(ctx, "", queueName, true, false, amqp.Publishing{
					ContentType:  "application/json",
					DeliveryMode: amqp.Persistent,
					Body:         body,
				})
				if err != nil {
					return err
				}
				ok, err := confirmation.WaitContext(ctx)
				if err == nil && !ok {
					return errors.New("broker rejected publication")
				}
				select {
				case <-returned:
					return errors.New("publication was unroutable")
				default:
					return err
				}
			})
			cancel()
			if err != nil {
				return err
			}
			if !found {
				pause(ctx, 300*time.Millisecond)
			}
		}
		return ctx.Err()
	})
}

func RunWorker(ctx context.Context, store *db.Store, url string, setReady func(bool), m *metrics.WorkerMetrics, transcribe func(context.Context, string) (string, error)) {
	supervise(ctx, "worker", url, func(ctx context.Context, ch *amqp.Channel) error {
		if err := ch.Qos(1, 0, false); err != nil {
			return err
		}
		messages, err := ch.ConsumeWithContext(ctx, queueName, "", false, false, false, false, nil)
		if err != nil {
			return err
		}
		setReady(true)
		defer setReady(false)
		for message := range messages {
			if ctx.Err() != nil {
				break
			}
			var task delivery
			if json.Unmarshal(message.Body, &task) != nil || task.TaskID == "" || task.Attempt < 1 {
				if err := message.Reject(false); err != nil {
					return err
				}
				continue
			}
			taskCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
			outcome, err := store.ProcessTask(taskCtx, task.TaskID, task.Attempt, func(audio string) (string, bool, error) {
				providerCtx, stop := context.WithTimeout(taskCtx, 30*time.Second)
				defer stop()
				start := time.Now()
				result, err := transcribe(providerCtx, audio)
				retry := !errors.Is(err, errNoTranscript)
				var rejected *interfaces.StatusError
				if errors.As(err, &rejected) {
					status := rejected.Resp.StatusCode
					retry = status == 408 || status == 429 || status >= 500
				}
				if m != nil {
					outcome := "success"
					if err != nil {
						outcome = "permanent_error"
						if retry {
							outcome = "retryable_error"
						}
					}
					m.ProviderDuration.WithLabelValues(outcome).Observe(time.Since(start).Seconds())
				}
				return result, retry, err
			})
			cancel()
			if errors.Is(err, db.ErrDeliveryPending) {
				// Confirmation can arrive just after the consumer receives the message.
				pause(ctx, 300*time.Millisecond)
				if err = message.Nack(false, true); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err // Closing the channel requeues its unacknowledged delivery.
			}
			if m != nil {
				switch outcome.Status {
				case "queued":
					m.RetriesScheduled.Inc()
				case "completed", "failed":
					if outcome.Age > 0 {
						m.TimeToResult.WithLabelValues(outcome.Status).Observe(outcome.Age.Seconds())
					}
				}
			}
			if err := message.Ack(false); err != nil {
				return err
			}
		}
		return errors.New("delivery channel closed")
	})
}

func supervise(ctx context.Context, name, url string, run func(context.Context, *amqp.Channel) error) {
	for ctx.Err() == nil {
		err := func() error {
			conn, err := connect(url)
			if err != nil {
				return err
			}
			defer conn.CloseDeadline(time.Now().Add(2 * time.Second))
			ch, err := conn.Channel()
			if err != nil {
				return err
			}
			if err := declare(ch); err != nil {
				return err
			}
			return run(ctx, ch)
		}()
		if ctx.Err() == nil {
			log.Printf("%s stopped (%T), reconnecting", name, err)
			pause(ctx, time.Second)
		}
	}
}

func pause(ctx context.Context, delay time.Duration) {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

# SpeechToText

Go API for transcribing audio URLs with Deepgram. PostgreSQL stores tasks and the outbox, RabbitMQ delivers jobs to the worker, and Redis stores login sessions.

## Run locally

Copy `.env.example` to `.env`, set `DEEPGRAM_API`, then run:

```sh
docker compose up --build -d postgres redis rabbitmq server worker prometheus grafana
```

[Swagger](http://localhost:8080/swagger/index.html) · [Prometheus](http://localhost:9090) · [Grafana](http://localhost:3000/d/speech-overview)

Register at `POST /register` and sign in at `POST /login`. Send the session cookie with `POST /audio` and a body like `{"audio":"https://example.com/clip.wav"}`. The response is `202 Accepted` with the task URL in `Location`. Poll that URL for the status and transcript. Reuse an `Idempotency-Key` to retry the same submission without creating another task.

## Checks

```sh
docker compose -p stt-test run --build --rm tests
docker compose -p stt-test down -v
```

The Go suite uses PostgreSQL, Redis and RabbitMQ with a transcription stub. The HTTP smoke script in `ops/` is intended for a disposable Compose stack with the worker stopped.

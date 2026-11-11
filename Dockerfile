FROM golang:1.24 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY src ./src
RUN CGO_ENABLED=0 go build -trimpath -o /dockerproject ./src/cmd

FROM build AS test
COPY tests ./tests
CMD ["go", "test", "-race", "-count=1", "./tests/go"]

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /dockerproject /dockerproject
COPY --from=build /app/src/db/migration ./src/db/migration
USER 10001
CMD ["/dockerproject"]

FROM golang:1.22.5 as builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o custom-scheduler

# Final stage
FROM debian:buster
WORKDIR /root/
COPY --from=builder /app/custom-scheduler .
CMD ["./custom-scheduler"]

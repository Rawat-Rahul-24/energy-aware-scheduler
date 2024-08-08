# FROM golang:1.22.5 as builder
# WORKDIR /app
# COPY . .
# RUN CGO_ENABLED=0 GOOS=linux go build -o custom-scheduler

# # Final stage
# FROM debian:buster
# WORKDIR /root/
# COPY --from=builder /app/custom-scheduler .
# CMD ["./custom-scheduler"]
############################for rpi use below

# Builder stage
FROM golang:1.22.5 as builder
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o custom-scheduler

# Final stage
FROM arm64v8/debian:buster
WORKDIR /root/
COPY --from=builder /app/custom-scheduler .
CMD ["./custom-scheduler"]

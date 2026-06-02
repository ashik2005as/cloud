FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/bid-service ./cmd/bid-service

FROM gcr.io/distroless/base-debian12
COPY --from=build /bin/bid-service /bid-service
EXPOSE 8082
ENTRYPOINT ["/bid-service"]

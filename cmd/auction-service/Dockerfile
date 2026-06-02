FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/auction-service ./cmd/auction-service

FROM gcr.io/distroless/base-debian12
COPY --from=build /bin/auction-service /auction-service
EXPOSE 8081
ENTRYPOINT ["/auction-service"]

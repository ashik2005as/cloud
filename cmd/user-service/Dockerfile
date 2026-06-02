FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/user-service ./cmd/user-service

FROM gcr.io/distroless/base-debian12
COPY --from=build /bin/user-service /user-service
EXPOSE 8080
ENTRYPOINT ["/user-service"]

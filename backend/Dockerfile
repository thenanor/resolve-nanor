FROM golang:1.26-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -o /out/triage ./cmd/triage
RUN CGO_ENABLED=0 go build -o /out/replyguard ./cmd/replyguard

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/api ./api
COPY --from=build /out/triage ./triage
COPY --from=build /out/replyguard ./replyguard
EXPOSE 3000
EXPOSE 3001
EXPOSE 3002
CMD ["./api"]

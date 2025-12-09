FROM golang:alpine AS build-stage

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /tags-app ./cmd/app/

FROM alpine AS build-release-stage

WORKDIR /

COPY --from=build-stage /tags-app /tags-app

COPY --from=build-stage /app/.env .

EXPOSE 8095

ENTRYPOINT ["./tags-app", "--config=.env", "--env=local"]
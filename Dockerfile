FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/retriv-api ./cmd/main.go

FROM gcr.io/distroless/static:nonroot

COPY --from=build /out/retriv-api /retriv-api

EXPOSE 8080

USER nonroot:nonroot

ENTRYPOINT ["/retriv-api"]
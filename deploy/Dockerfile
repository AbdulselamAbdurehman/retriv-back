# Production/full-docker build for the retriv API — single static binary.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/retriv ./cmd

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/retriv /retriv
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/retriv"]

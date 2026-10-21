FROM golang:1.25.0 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/minipay ./cmd/minipay

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/minipay /minipay
EXPOSE 8080
ENTRYPOINT ["/minipay"]
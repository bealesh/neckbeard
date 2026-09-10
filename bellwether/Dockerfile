# bellwether — reference app for the neckbeard release matrix.
# Multi-stage: static binary in a minimal, nonroot runtime image.
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG APP_VERSION=0.1.0
RUN CGO_ENABLED=0 go build -ldflags="-X main.version=${APP_VERSION}" -o /bellwether ./bellwether

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bellwether /bellwether
EXPOSE 8080
ENTRYPOINT ["/bellwether"]
CMD ["serve"]

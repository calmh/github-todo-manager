FROM golang:latest AS builder
WORKDIR /app
COPY . /app
RUN CGO_ENABLED=0 go build -v

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/github-todo-manager /bin/github-todo-manager
ENTRYPOINT [ "/bin/github-todo-manager" ]

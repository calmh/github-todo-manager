FROM golang:latest AS builder
WORKDIR /app
COPY . /app
RUN CGO_ENABLED=0 go build -v

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /app/github-todo-recurrence /bin/github-todo-recurrence
ENTRYPOINT [ "/bin/github-todo-recurrence" ]

FROM golang:1.26-alpine

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /service ./users/internal/cmd/users-api

ENTRYPOINT ["/service"]

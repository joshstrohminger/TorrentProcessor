FROM golang:1.23-alpine3.20 as build

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -v -o /usr/local/bin/app .

FROM scratch

COPY --from=build /usr/local/bin/app /app
ENTRYPOINT [ "/app" ]
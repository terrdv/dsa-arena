# dsa-arena

## Running the server

Requires [Go](https://go.dev/dl/) 1.22+.

```sh
cd server
go run ./cmd/server
```

The server listens on port `8080` by default. To use a different port, set the `PORT` environment variable:

```sh
PORT=3000 go run ./cmd/server
```

Verify it's running:

```sh
curl localhost:8080/health
# ok
```

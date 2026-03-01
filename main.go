package main

import "github.com/joshstrohminger/TorrentProcessor/cmd"

//go:generate go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
//go:generate go install google.golang.org/protobuf/cmd/protoc-gen-go
//go:generate protoc --go_out=internal/api --go_opt=paths=source_relative --go-grpc_out=internal/api --go-grpc_opt=paths=source_relative --go_opt=default_api_level=API_OPAQUE api.proto

func main() {
	cmd.Execute()
}

module github.com/duragraph/duragraph-examples/go/01-hello-world

go 1.22

toolchain go1.23.4

require github.com/duragraph/duragraph/go-sdk v0.0.0-00010101000000-000000000000

require (
	github.com/klauspost/compress v1.17.11 // indirect
	github.com/nats-io/nats.go v1.34.0 // indirect
	github.com/nats-io/nkeys v0.4.9 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.31.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
)

replace github.com/duragraph/duragraph/go-sdk => ../../../go-sdk

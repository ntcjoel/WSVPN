module wsvpn

go 1.24

toolchain go1.24.4

require (
	github.com/gorilla/websocket v1.5.1
	github.com/quic-go/quic-go v0.59.0
	github.com/songgao/water v0.0.0-20200317203138-2b4b6d7c09d8
	github.com/vishvananda/netlink v1.2.1-beta.2
	golang.org/x/sys v0.35.0
	golang.zx2c4.com/wintun v0.0.0-20230126152724-0fa3db229ce2
)

require (
	github.com/vishvananda/netns v0.0.0-20200728191858-db3c7e526aae // indirect
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.43.0 // indirect
)

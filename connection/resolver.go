// Package connection provides network connection management for Pod-OS clients.
package connection

import (
	"log"
	"net"

	"github.com/PointOfData/pod-os-go-client/errors"
)

// Resolve resolves a network address.
func Resolve(network, address string) (string, *errors.GatewayDError) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		addr, err := net.ResolveTCPAddr(network, address)
		if err == nil {
			return addr.String(), nil
		}
		return "", errors.ErrResolveFailed.Wrap(err)
	case "udp", "udp4", "udp6":
		addr, err := net.ResolveUDPAddr(network, address)
		if err == nil {
			return addr.String(), nil
		}
		return "", errors.ErrResolveFailed.Wrap(err)
	case "unix", "unixgram", "unixpacket":
		addr, err := net.ResolveUnixAddr(network, address)
		if err == nil {
			return addr.String(), nil
		}
		return "", errors.ErrResolveFailed.Wrap(err)
	default:
		log.Panicf("network is not supported: %s", address)
		return "", errors.ErrNetworkNotSupported
	}
}


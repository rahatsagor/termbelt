//go:build windows

package core

import (
	"net"
	"net/netip"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// systemResolvers reads DNS servers from active network adapters, because
// Windows has no /etc/resolv.conf.
func systemResolvers() []string {
	size := uint32(15 << 10)
	var buffer []byte
	for attempt := 0; attempt < 3; attempt++ {
		buffer = make([]byte, size)
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, windows.GAA_FLAG_SKIP_ANYCAST|windows.GAA_FLAG_SKIP_MULTICAST|windows.GAA_FLAG_SKIP_FRIENDLY_NAME, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])), &size)
		if err == nil {
			break
		}
		if err != windows.ERROR_BUFFER_OVERFLOW {
			return nil
		}
		buffer = nil
	}
	if buffer == nil {
		return nil
	}
	seen := map[string]bool{}
	servers := []string{}
	for adapter := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0])); adapter != nil; adapter = adapter.Next {
		if adapter.OperStatus != windows.IfOperStatusUp {
			continue
		}
		for dns := adapter.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			sockaddr, err := dns.Address.Sockaddr.Sockaddr()
			if err != nil {
				continue
			}
			var ip netip.Addr
			switch v := sockaddr.(type) {
			case *syscall.SockaddrInet4:
				ip = netip.AddrFrom4(v.Addr)
			case *syscall.SockaddrInet6:
				ip = netip.AddrFrom16(v.Addr)
			default:
				continue
			}
			// Site-local fec0::/10 entries are Windows placeholders, not resolvers.
			if ip.Is6() && ip.As16()[0] == 0xfe && ip.As16()[1]&0xc0 == 0xc0 {
				continue
			}
			if !ip.IsValid() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
				continue
			}
			address := net.JoinHostPort(ip.String(), "53")
			if !seen[address] {
				seen[address] = true
				servers = append(servers, address)
			}
		}
	}
	return servers
}

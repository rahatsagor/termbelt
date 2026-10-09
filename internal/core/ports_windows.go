//go:build windows

package core

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
)

const tcpTableOwnerPIDListener = 3

type tcpRowOwnerPID struct {
	State, LocalAddr, LocalPort, RemoteAddr, RemotePort, OwningPID uint32
}

type tcp6RowOwnerPID struct {
	LocalAddr               [16]byte
	LocalScopeID, LocalPort uint32
	RemoteAddr              [16]byte
	RemoteScopeID           uint32
	RemotePort, State, PID  uint32
}

func extendedTCPTable(family uint32) ([]byte, error) {
	size := uint32(0)
	for attempt := 0; attempt < 4; attempt++ {
		var buffer []byte
		var pointer uintptr
		if size > 0 {
			buffer = make([]byte, size)
			pointer = uintptr(unsafe.Pointer(&buffer[0]))
		}
		result, _, _ := procGetExtendedTcpTable.Call(pointer, uintptr(unsafe.Pointer(&size)), 1, uintptr(family), tcpTableOwnerPIDListener, 0)
		switch windows.Errno(result) {
		case 0:
			return buffer, nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			size += 4096
		default:
			return nil, fmt.Errorf("GetExtendedTcpTable: %w", windows.Errno(result))
		}
	}
	return nil, fmt.Errorf("GetExtendedTcpTable: table kept growing")
}

func networkPort(value uint32) int { return int(value&0xff)<<8 | int(value>>8&0xff) }

func windowsListeners() ([]Listener, error) {
	if err := procGetExtendedTcpTable.Find(); err != nil {
		return nil, err
	}
	names := map[uint32]string{}
	process := func(pid uint32) string {
		if name, ok := names[pid]; ok {
			return name
		}
		name := ""
		if pid == 4 {
			name = "System"
		} else if handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid); err == nil {
			buffer := make([]uint16, windows.MAX_PATH)
			size := uint32(len(buffer))
			if windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size) == nil {
				name = filepath.Base(windows.UTF16ToString(buffer[:size]))
			}
			windows.CloseHandle(handle)
		}
		names[pid] = name
		return name
	}
	listeners := []Listener{}
	if table, err := extendedTCPTable(windows.AF_INET); err != nil {
		return nil, err
	} else if len(table) >= 4 {
		count := *(*uint32)(unsafe.Pointer(&table[0]))
		rows := unsafe.Slice((*tcpRowOwnerPID)(unsafe.Pointer(&table[4])), count)
		for _, row := range rows {
			address := netip.AddrFrom4([4]byte{byte(row.LocalAddr), byte(row.LocalAddr >> 8), byte(row.LocalAddr >> 16), byte(row.LocalAddr >> 24)})
			listeners = append(listeners, Listener{Port: networkPort(row.LocalPort), Address: address.String(), Process: process(row.OwningPID), PID: int(row.OwningPID)})
		}
	}
	if table, err := extendedTCPTable(windows.AF_INET6); err == nil && len(table) >= 4 {
		count := *(*uint32)(unsafe.Pointer(&table[0]))
		rows := unsafe.Slice((*tcp6RowOwnerPID)(unsafe.Pointer(&table[4])), count)
		for _, row := range rows {
			listeners = append(listeners, Listener{Port: networkPort(row.LocalPort), Address: netip.AddrFrom16(row.LocalAddr).String(), Process: process(row.PID), PID: int(row.PID)})
		}
	}
	return listeners, nil
}

package core

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type Listener struct {
	Port    int    `json:"port"`
	Address string `json:"address"`
	Process string `json:"process,omitempty"`
	PID     int    `json:"pid,omitempty"`
}

func LocalPorts(ctx context.Context, r Request) (Result, error) {
	filter := 0
	if port := strings.TrimSpace(r.Opt("port", "")); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return Result{}, fmt.Errorf("port must be 1–65535")
		}
		filter = n
	}
	listeners, source, err := listListeners(ctx, filter)
	if err != nil {
		return Result{}, err
	}
	seen := map[string]bool{}
	unique := []Listener{}
	for _, l := range listeners {
		key := fmt.Sprintf("%d|%s|%d", l.Port, l.Address, l.PID)
		if (filter == 0 || l.Port == filter) && !seen[key] {
			seen[key] = true
			unique = append(unique, l)
		}
	}
	sort.SliceStable(unique, func(i, j int) bool {
		if unique[i].Port != unique[j].Port {
			return unique[i].Port < unique[j].Port
		}
		return unique[i].Address < unique[j].Address
	})
	table := &Table{Headers: []string{"PORT", "PROCESS", "PID", "ADDRESS"}}
	hidden := false
	for _, l := range unique {
		pid := ""
		if l.PID > 0 {
			pid = strconv.Itoa(l.PID)
		}
		if l.Process == "" && l.PID == 0 {
			hidden = true
		}
		table.Rows = append(table.Rows, []string{strconv.Itoa(l.Port), nonempty(l.Process, "—"), nonempty(pid, "—"), l.Address})
	}
	summary := fmt.Sprintf("%d visible listeners", len(unique))
	if filter != 0 {
		summary = fmt.Sprintf("%d listeners on port %d", len(unique), filter)
	}
	notes := []string{"Source: " + source + ". Listeners owned by other users may be hidden or lack process details."}
	if hidden {
		notes = append(notes, "Run with elevated privileges to see the process behind every listener.")
	}
	return Result{Title: "Local listening ports", Summary: summary, Table: table, Notes: notes, Data: unique}, nil
}

func listListeners(ctx context.Context, filter int) ([]Listener, string, error) {
	switch runtime.GOOS {
	case "linux":
		if _, err := exec.LookPath("ss"); err == nil {
			out, err := exec.CommandContext(ctx, "ss", "-ltnp").Output()
			if err == nil {
				return parseSS(string(out)), "ss -ltnp", nil
			}
		}
		listeners, err := procNetListeners()
		return listeners, "/proc/net/tcp", err
	case "windows":
		listeners, err := windowsListeners()
		return listeners, "Windows TCP table", err
	default:
		args := []string{"-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn"}
		if filter != 0 {
			args[1] = "-iTCP:" + strconv.Itoa(filter)
		}
		out, err := exec.CommandContext(ctx, "lsof", args...).Output()
		if err != nil {
			if ex, ok := err.(*exec.ExitError); ok && ex.ExitCode() == 1 && len(out) == 0 {
				return nil, "lsof", nil
			}
			return nil, "", fmt.Errorf("local port inspection requires lsof: %w", err)
		}
		return parseLsof(string(out)), "lsof", nil
	}
}

func splitListenAddress(address string) (string, int, bool) {
	index := strings.LastIndex(address, ":")
	if index < 0 {
		return "", 0, false
	}
	port, err := strconv.Atoi(address[index+1:])
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	host := strings.Trim(address[:index], "[]")
	if host == "*" {
		host = "0.0.0.0"
	}
	return host, port, true
}

func parseLsof(output string) []Listener {
	listeners := []Listener{}
	pid, process := 0, ""
	for _, line := range strings.Split(output, "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
		case 'c':
			process = line[1:]
		case 'n':
			if host, port, ok := splitListenAddress(line[1:]); ok {
				listeners = append(listeners, Listener{Port: port, Address: host, Process: process, PID: pid})
			}
		}
	}
	return listeners
}

var ssProcess = regexp.MustCompile(`\("([^"]*)",pid=(\d+)`)

func parseSS(output string) []Listener {
	listeners := []Listener{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] == "State" {
			continue
		}
		host, port, ok := splitListenAddress(fields[3])
		if !ok {
			continue
		}
		// Strip interface scope, e.g. 127.0.0.53%lo.
		if i := strings.IndexByte(host, '%'); i >= 0 {
			host = host[:i]
		}
		matches := ssProcess.FindAllStringSubmatch(strings.Join(fields[5:], " "), -1)
		if len(matches) == 0 {
			listeners = append(listeners, Listener{Port: port, Address: host})
		}
		for _, m := range matches {
			pid, _ := strconv.Atoi(m[2])
			listeners = append(listeners, Listener{Port: port, Address: host, Process: m[1], PID: pid})
		}
	}
	return listeners
}

func procNetListeners() ([]Listener, error) {
	listeners := []Listener{}
	found := false
	for _, file := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		found = true
		listeners = append(listeners, parseProcNet(bufio.NewScanner(f))...)
		f.Close()
	}
	if !found {
		return nil, fmt.Errorf("local port inspection requires ss or /proc/net/tcp")
	}
	return listeners, nil
}

func parseProcNet(scanner *bufio.Scanner) []Listener {
	listeners := []Listener{}
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 || fields[3] != "0A" {
			continue
		}
		hexAddress, hexPort, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(hexPort, 16, 16)
		raw, hexErr := hex.DecodeString(hexAddress)
		if err != nil || hexErr != nil || (len(raw) != 4 && len(raw) != 16) {
			continue
		}
		// The kernel prints each 32-bit word in host (little-endian) order.
		for i := 0; i+4 <= len(raw); i += 4 {
			raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
		}
		ip, _ := netip.AddrFromSlice(raw)
		listeners = append(listeners, Listener{Port: int(port), Address: ip.Unmap().String()})
	}
	return listeners
}

//go:build !windows

package core

func systemResolvers() []string { return resolvConfServers() }

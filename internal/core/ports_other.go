//go:build !windows

package core

func windowsListeners() ([]Listener, error) { return nil, nil }

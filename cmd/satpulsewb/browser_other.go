//go:build !darwin && !linux && !windows

package main

import "errors"

func launchBrowser(string) error {
	return errors.New("browser launch is not supported")
}

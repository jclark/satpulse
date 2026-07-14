//go:build !darwin && !windows

package main

import "errors"

func launchBrowser(string) error {
	return errors.New("browser launch is not supported")
}

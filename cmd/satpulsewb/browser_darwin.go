package main

import "os/exec"

func launchBrowser(url string) error {
	return exec.Command("open", url).Run()
}

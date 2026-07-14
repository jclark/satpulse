package main

import "os/exec"

func launchBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Run()
}

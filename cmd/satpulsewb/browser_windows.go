package main

import "golang.org/x/sys/windows"

func hasGraphicalSession() bool {
	return true
}

// launchBrowser opens the URL by calling ShellExecuteW in-process
// rather than spawning rundll32, so the token-bearing URL appears on
// no command line of ours. The browser's own command line does
// receive the URL; process DACLs keep it unreadable to other
// non-administrator users.
func launchBrowser(url string) error {
	verb, err := windows.UTF16PtrFromString("open")
	if err != nil {
		return err
	}
	u, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return err
	}
	return windows.ShellExecute(0, verb, u, nil, nil, windows.SW_SHOWNORMAL)
}

// launchBrowserLeaksURL reports whether launchBrowser exposes the URL to
// other local users. False on Windows: the URL lands on the browser's own
// command line, but process DACLs deny cross-user command-line reads.
const launchBrowserLeaksURL = false

# Setup for Web front end

1. Install nodejs (which includes npm). I recommend using [NodeSource binary distributions](https://github.com/nodesource/distributions). I used version 22.
2. Run `npm install` to install the needed dependencies.
3. Run `go generate` to regenerate the `.js` file that is embedded in the Go binary.
4. Install typescript globally using `sudo npm install -g typescript`; you can then use `npm run typecheck` to type-check the TypeScript code.

You can test the dashboard by doing `npm run dashtestbuild` and then opening `test-dashboard.html` in your browser.
You can also open e.g. `test-dashboard.html?g=GE` to show only satellites with a SVID starting with G or E (i.e. GPS or Galileo).
Use `test-dashboard.html?angles=0` to simulate UBX-NAV-SIG only mode (no look angles) - this shows signal graph but no sky view.

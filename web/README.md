# Setup for Web front end

1. Install nodejs (which includes npm). I recommend using [NodeSource binary distributions](https://github.com/nodesource/distributions). For example, on Ubuntu, you can do this:

    ```bash
    curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
    sudo apt-get install nodejs
    ```
2. Run `npm install` to install the needed dependencies.
3. Run `go generate` to regenerate the `.js` file that is embedded in the Go binary.
4. Install typescript globally using `sudo npm install -g typescript`; you can then use `npm run typecheck` to type-check the TypeScript code.
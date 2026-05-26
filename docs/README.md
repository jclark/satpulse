This directory is designed to be served with GitHub Pages.

## Local preview

You need Ruby 3.3 or newer (required by `github-pages` 232, which pins Jekyll 3.10).

On macOS:

```
brew install ruby
```

On Debian 13 (trixie) or newer, install Ruby, Bundler, and the headers
needed to compile the native-extension gems (nokogiri, ffi, eventmachine,
http_parser.rb, commonmarker) from source:

```
sudo apt install --no-install-recommends \
  ruby ruby-dev ruby-bundler build-essential \
  zlib1g-dev libffi-dev libyaml-dev libssl-dev \
  libxml2-dev libxslt1-dev pkg-config
```

Debian's own `ruby-<gem>` packages are not used: most don't match the
versions `github-pages` 232 pins, and the vendored Bundler layout below
wouldn't pick them up anyway.

Then in this directory, install the gems into a project-local
`vendor/bundle/` so nothing leaks into your user or system gem path:

```
bundle config set --local path 'vendor/bundle'
bundle install
```

Then run:

```
bundle exec jekyll serve
```

While this is running, visit [`http://localhost:4000`](http://127.0.0.1:4000/).

To make the preview reachable from other machines on your network, bind
to all interfaces:

```
bundle exec jekyll serve --host 0.0.0.0
```

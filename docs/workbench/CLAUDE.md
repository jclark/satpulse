# Editing the workbench pages

One page per tab of satpulsewb. Each page shows screenshots of its tab
with the theme's gallery include, the same way the blog post
introducing the workbench does. CLAUDE.md files are excluded from the
published web site via the `exclude` list in `docs/_config.yml`.

## Gallery

The front matter has `toc: false` and a `<page>_gallery` list, one
entry per image, each with `url`, `image_path`, `alt` and `title`:

```yaml
monitor_gallery:
  - url: /assets/images/wb-f9p-monitor.png
    image_path: /assets/images/wb-f9p-monitor.png
    alt: "Monitor tab with a ZED-F9P: clock, fix summary, map and sky view"
    title: "Monitor tab with a ZED-F9P"
```

The body places it with `{% include gallery id="monitor_gallery" %}`.
`alt` describes what is in the image; `title` is the short caption.

## Images

Images live in `docs/assets/images/` and are named
`wb-<run>-<shot>.png`, where `<run>` identifies the receiver the shot
was taken with (f9p, g5, allystar, casic, ...) and `<shot>` the view.
Different pages deliberately show different receiver models: the
receiver on each page is chosen for what that tab can show with it
(an RTK fix, a built-in message file, a section that greys out), not
for uniformity. Every image under a page's gallery must exist, and no
`wb-*.png` should be left unreferenced.

New or replacement images are taken with the `workbench-screenshots`
skill, which reads the galleries to find out what each page currently
shows and takes the shots against a live satpulsewb.

# hugo-reticulum

Serve a Hugo site as Micron pages over Reticulum. Hugo renders the site,
this module supplies Micron templates, and the `hugo-reticulum` tool
converts the Markdown bodies to Micron and serves the result to NomadNet
clients.

## Install

You need Hugo and Go 1.27 or later.

```sh
go install github.com/Quad4-Software/hugo-reticulum/cmd/hugo-reticulum@latest
```

Then add the module to your site config:

```toml
# hugo.toml
[module]
  [[module.imports]]
    path = "github.com/Quad4-Software/hugo-reticulum"

[outputs]
home = ["Micron"]
page = ["Micron"]
section = ["Micron"]
taxonomy = ["Micron"]
term = ["Micron"]
```

## Quick start

```sh
# 1. Build the Micron site and convert the Markdown bodies
hugo-reticulum build

# 2. Serve it over Reticulum
hugo-reticulum serve
```

Run `hugo-reticulum serve --watch` to rebuild when content changes.

The tool looks for `hugo-reticulum.toml` in the site directory and walks
up from there. With no config file it uses sensible defaults: Hugo builds
into `public/`, `.mu` files are rewritten in place, and `static/` is
served under `/file/`.

## How it works

Templates emit page chrome (navigation, headings, footers) as raw Micron.
Page bodies are emitted as Markdown between `<!--md-->` and
`<!--/md-->` sentinel lines, with shortcodes already expanded. The tool
converts each sentinel region with a goldmark based Markdown to Micron
converter and rewrites links to `:/page/...` and `:/file/...` paths that
match the built site.

You can override any template or partial using the standard Hugo lookup
order. The defaults live in `layouts/` at the module root.

## Configuration

### hugo-reticulum.toml

```toml
[hugo]
path = "hugo"          # hugo binary, default "hugo"
args = []              # extra arguments appended to the hugo command

[site]
source_dir = "."       # Hugo site directory
public_dir = "public"  # Hugo publish directory inside source_dir
output_dir = ""        # where final .mu files go, empty means in place
static_dir = "static"  # directory served under /file/
in_place = true        # rewrite .mu files inside public_dir

[reticulum]
config_path = ""              # Reticulum-Go config file
identity_path = ""            # Reticulum identity file
node_name = ""                # announce display name
announce_interval_minutes = 360 # periodic announce period, 0 announces once
pages_dir = ""                # served under /page/, default is public_dir
files_dir = ""                # served under /file/, default is static_dir
page_refresh_seconds = 0      # rescan pages on an interval, 0 disables
file_refresh_seconds = 0      # rescan files on an interval, 0 disables
disable_page_stats = false    # turn off the page view counter
node_hash = ""                # prefix links as "<hash>:/page/..."

[markdown]
max_width = 100        # table and wrap width budget
link_style = "both"    # plain, underline, bold, both, or color
link_color = ""        # hex color for link_style = "color"
image_policy = "link"  # link, alt, or skip
table_align = "center" # "", "left", "center", or "right"

log = "info" # debug, info, warn, or error
```

`image_policy` values: `link` emits Micron image links, `alt` emits the
alt text, `skip` drops images.

### params.reticulum (hugo.toml)

These keys control the Micron chrome the templates emit.

| Key | Default | Meaning |
| --- | --- | --- |
| `node_name` | `""` | Node name used when announcing. The serve command prefers `hugo-reticulum.toml`. |
| `bg` | `""` | Page background color, emitted as `#!bg=`. Accepts `#rgb` or `#rrggbb`. |
| `fg` | `""` | Page foreground color, emitted as `#!fg=`. |
| `max_width` | `100` | Width budget passed to the converter for tables and wrapped text. |
| `show_nav` | `true` | Show the navigation block. |
| `nav_menu` | `"main"` | Menu name used for navigation links. |
| `footer` | `""` | Footer text shown at the bottom of each page. |
| `link_style` | `"both"` | Link decoration in converted bodies: `plain`, `underline`, `bold`, `both`, `color`. |
| `link_color` | `""` | Hex color used when `link_style` is `color`. |
| `cache_ttl` | `""` | Cache hint emitted as a `#!c=` header line. |
| `wrap_text` | `true` | Reserved for template use. Line wrapping happens in the converter via `max_width`. |

### Commands

```sh
hugo-reticulum convert [flags] <file.md | ->
hugo-reticulum build [flags]
hugo-reticulum serve [flags]
```

`convert` flags: `--max-width`, `--link-style`, `--link-color`,
`--image-policy`, `--table-align`, `--scope`, `--node-hash`, `-o`.

`build` and `serve` flags: `--source`, `--config`, `--hugo`,
`--output`, `--static`, `--in-place`. `serve` adds `--watch`,
`--watch-interval`, `--identity`, `--reticulum-config`,
`--page-refresh`, `--file-refresh`, and `--no-page-stats`.

## Notes

- The node runs its own Reticulum transport through Reticulum-Go's
  pageserver. It does not attach to an existing shared Reticulum instance.
- Tables render as aligned monospace blocks. Code blocks render as Micron
  literal blocks. Fenced code with the `rawmu` language passes Micron
  source through untouched.
- Relative Markdown links resolve against the built `.mu` tree, so links
  to `page.md` become `:/page/...` paths that work from any page.

## License

MIT. See LICENSE. This project depends on Reticulum-Go, which is
published under the Reticulum License.

## The Reticulum-Go docs site

`site/` is the Reticulum-Go documentation site. It mounts `../docs/en` as
its content through `module.mounts`, so the Markdown source lives in the
repository docs tree and carries no front matter. Page titles come from
the first `#` heading (`site/layouts/partials/doc-title.html`) and the
sidebar order comes from `site/data/docsnav.toml`, which mirrors the
reading order in `docs/en/README.md`.

Build everything:

```sh
cd website
go run ./cmd/docs-bundle          # writes site/static/downloads/*
cd site && hugo --source . && ../bin/hugo-reticulum build
```

`task docs-site` from the repository root runs the same pipeline.
`hugo server --source site` previews the HTML build on port 1313 and
`bin/hugo-reticulum serve` run from `site/` serves the Micron pages over
Reticulum.

### Downloads

`cmd/docs-bundle` reads the nav order in `site/data/docsnav.toml` and
produces a Markdown ZIP with an `INDEX.md`, a combined print HTML, a PDF
rendered with headless Chromium, and a hand assembled EPUB 3 in
`site/static/downloads` (gitignored). Versioned files plus stable aliases
are written; the version comes from the top released heading in the root
CHANGELOG. Pass `-require-pdf` to fail when no Chromium is found. The
HTML site links the files from the top bar menu and `/downloads/`; the
Micron pages mention them under `/file/downloads/`.

### Theme and JavaScript

The site supports light, dark and system themes. A small inline script in
`<head>` applies the saved `localStorage` choice (or `?theme=` query)
before first paint so there is no flash. `site/static/js/site.js` adds
the theme menu, a code copy button, the mobile sidebar toggle and menu
keyboard handling. Without JavaScript the theme falls back to
`prefers-color-scheme`, menus stay usable as `<details>` elements, and the
sidebar stays visible. Nothing is fetched from the network: no web fonts,
no CDN assets.

### Icons

Icons are vendored SVGs from the Lucide set (ISC license) under
`site/assets/icons`, rendered through the `icon` partial so they inherit
`currentColor`. The vendored copy, its license, the exact version and the
tarball hash live in `site/assets/icons/`. To refresh: download the
matching `lucide-static` release tarball, copy the needed SVGs over the
existing ones, and update `site/assets/icons/README.md` with the new
version and sha256.

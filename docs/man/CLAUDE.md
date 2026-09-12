# Writing man pages

The `.md` files here are pandoc markdown, converted to man pages by
the Makefile (`pandoc -s -t man`). Title, section, and author come
from the filename (`name.section.md`), so files have no front matter:
they start directly with `# NAME`. CLAUDE.md files are excluded from
the published web site via the `exclude` list in `docs/_config.yml`.

## What a man page is

A man page states the command's contract at the level a user relies
on. It is reference documentation for users: not a tutorial, not a
detailed specification of the behaviour, and not an implementation
document. It does not try to exhaustively describe what the command
does, and behaviour it does not mention is not promised. What the
program is built from - internal names, code structure, design
concepts - never appears; neither does motivation for a feature's
existence.

Brevity is a key virtue. Every sentence must tell the user
something needed to use the command; a fact that is self-evident,
incidental, or of no consequence to the user is left out even
though it is true and checkable. Judge a page by what the user
needs, not by how completely it matches the code: a page is wrong
for misstating behaviour, never for omitting it.

Above all, be consistent in style with the existing man pages.

## Content

- Document observable behavior only: inputs, outputs, defaults,
  constraints, side effects, exit codes. An EXIT STATUS entry names
  the kinds of failure a code covers, not every path that produces
  it.
- Say what happens in the cases the reader would otherwise have to
  guess: when an option is omitted, what is skipped. A guarantee of
  what the program does *not* do can be content ("does not rescan,
  re-encode, re-checksum").
- A constraint gets a sentence only when the reader would otherwise
  guess wrong; do not enumerate every invalid option combination or
  error case.
- Each fact lives where the user will look for it - an option's
  behavior at the option - and is cross-referenced from elsewhere,
  not repeated ("as with **\-\-gnss**", "see ENVIRONMENT").
- A one-clause "since ..." may justify a constraint or default when
  it helps the reader predict behavior; nothing longer, and never
  for the feature itself.
- DESCRIPTION carries what the program is, its modes, and
  operational context (how it is typically run, what to expect at
  startup); OPTIONS entries stay strictly factual.
- Be exact where exactness is checkable: units, ranges, formats,
  which values are accepted, what the default is. Wording must match
  the semantics precisely (a receiver is connected; an option
  overrides an environment variable).
- Use the vocabulary the pages have already established for a
  concept, and expand an abbreviation at first use in a page:
  "pulse-per-second (PPS)".

## Writing style

- Declarative present tense with the program or command as subject:
  "**satpulsed** is a daemon that ...", "The **scan** command reads
  a GPS packet byte stream and writes a JSONL packet log".
- Option descriptions open with a verb stating the effect ("Show
  ...", "Set ...", "Restrict ...", "Enable ..."); an option that
  just supplies a value can be a bare noun phrase ("Path to the
  serial device."). ENVIRONMENT and FILES entries are noun phrases.
- Short sentences, one fact each, one sentence per source line.
  After the opening sentence: value syntax, then defaults, then
  constraints and interactions.
- Say each fact once; never restate it in a mirror clause ("reads
  from serial ports but never writes to them", not "only reads from
  serial ports; it never transmits to a connected device").
- Stock phrasings for recurring facts: "The default is 2.",
  "Requires **\-\-gnss**.", "Cannot be combined with ...", "Applies
  only when ... is specified.", "This option may be repeated.",
  "The value is case-insensitive.", "Use **\-** as *file* to read
  from standard input."
- Avoid addressing the reader; describe the program's behavior
  rather than the user's actions.

## Sections

Sections are `#` headings in capitals. NAME, SYNOPSIS, DESCRIPTION,
and OPTIONS come first in that order (satpulsetool.1 has COMMANDS
before OPTIONS); SEE ALSO is always last; EXAMPLES goes near the
end. ENVIRONMENT, FILES, and EXIT STATUS appear only when the page
has something to say; their relative order varies between existing
pages, so follow the page being edited. Page-specific sections (e.g.
convobs's HEADER FILE FORMAT) go between OPTIONS and EXAMPLES.

NAME is `name - summary`: a lowercase phrase, no trailing period; a
verb phrase for commands ("convert a packet byte stream to a JSONL
packet log"), a noun phrase for satpulsed ("integrated GPS daemon").

## SYNOPSIS

One synopsis listing every option. Subcommand pages start
`**satpulsetool** [*global options*] **cmd**`. Optional items are
bracketed, alternatives separated by `\|`, e.g.
`[**\-h**\|**\-\-help**]`. Continuation lines end with `\` and start
with `&nbsp;&nbsp;&nbsp;&nbsp;`. Keep the SYNOPSIS in sync when
adding or removing options.

## Markup

- Bold: program names, option names, literal option values, man page
  references (**satpulsed(8)**), environment variable names.
- Italic: placeholders (*path*, *seconds*, *file*), including in
  option descriptions when referring to the argument.
- Backticks: TOML keys and tables (`vendor`, `[gps]`), packet-log
  field names (`tag`, `bin`), literal strings and code.
- Escape hyphens in option names everywhere: `**\-\-vendor**`,
  `**\-h**`.
- Definition lists are the pandoc form: the term on its own line,
  the description on the next line starting `: `. Used for options,
  commands, environment variables, files, and exit statuses.

## OPTIONS

An entry's term is `**\-x**, **\-\-long** *arg*` (short form first).
The description states what the option does, then value syntax and
constraints, then interactions, each as its own sentence, one
sentence per source line:

- Defaults as "The default is 2." (or "(default: 2000)" inline).
- Constraints as "Requires **\-\-gnss**.", "Cannot be combined with
  **\-\-msg\-file**.", "Applies only when **\-\-perout** is
  specified.", "This option may be repeated."
- An option taking a comma-separated list of keywords documents the
  keywords as a nested definition list indented two spaces (see
  **\-\-pvt\-out** in satpulsetool-gps.1.md).

Long OPTIONS sections are grouped, either with `##` subsections
(satpulsetool-gps.1.md) or with narrative lead-ins ("The following
options control ..."), and every option belongs to a group.

## EXAMPLES

Each example is a one-line description ending with a colon, then the
command in a 4-space-indented block. Use realistic values (real
device paths, plausible baud rates). Order from simplest to most
elaborate; multi-step examples may include supporting file content.

## satpulse.toml.5.md

The config-file page uses its own idiom: each table gets a `##`
section, and keys are a `*` bullet list of the form
`` `key` - a <type> giving ... ``, with clauses joined by
semicolons rather than separate sentences. Follow it exactly when
adding keys; per `time/app/daemon/CLAUDE.md`, TOML option changes
must update this page in the same change.

## SEE ALSO

Bold comma-separated references: SatPulse's own pages first, then
external ones (**systemd(1)**, **ptp4l(8)**, **jq(1)**).

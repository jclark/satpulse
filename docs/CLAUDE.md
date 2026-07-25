# Website content style

The site has one voice: a single engineer writing from direct
experience. Before writing or extending a page, read an existing page
of the same kind and match it:

- setup task page: `setup/phc.md`
- concept page: `intro/timing.md`
- hardware selection page: `hardware/index.md`
- survey page: `intro/other-software.md`

## Voice

- "I" is the author's experience and opinion ("I suggest making it a
  separate file", "the only suitable model I have found"). Never
  write "I" for anything the author has not actually done or tested.
- "you" is the reader doing the task. "we" appears occasionally when
  walking through reasoning ("we are trying to solve two problems").
- Factual, specific, quantified: model numbers, prices, versions,
  exact paths and flags. Where a fact will age, anchor it in time
  ("At the time of writing, 2026Q2, ...").
- State limitations plainly and immediately ("This is imprecise but
  is useful when...", "The original author no longer maintains it").

## Structure

- Setup pages start with the first action, not with scene-setting.
  No closing summary; the page ends when the task does.
- After each config snippet or command, one or two sentences say what
  it does ("This will make satpulsed send timing samples to chrony").
- Content lives in prose paragraphs; bullets only for real
  enumerations (options, product lists), never for narrative.

## Correctness

- Never state a capability claim that is not verified against the man
  pages (`man/`) or the code. Man pages are the reference: link to
  them rather than restating their content.
- A visible TODO is acceptable; an invented or wrong claim is not.
  When information is missing, write `TODO: ...` and move on.

## Agent tells to avoid

These patterns mark text as machine-written; none of them appear on
the site:

- marketing adjectives: powerful, seamless, robust, comprehensive,
  effortless, cutting-edge; and verbs like leverage, streamline
- adjective triads ("fast, flexible, and reliable")
- bold lead-in bullets ("**Fast:** ...")
- openers that announce the page ("In this guide, we'll...") and
  wrap-ups ("You now have a fully working...")
- filler hedges ("It's worth noting that", "Keep in mind that")
- exclamation marks

Instead of: "SatPulse's powerful configuration engine makes receiver
setup effortless."
Write: "satpulsed configures the receiver at startup: with a u-blox
or Unicore receiver, it enables the messages that PHC synchronization
needs. Changes are made only in RAM and are undone if the receiver is
power cycled."

## Mechanics

- Headings in sentence case.
- Break lines at sentence boundaries, and at clause boundaries in
  long sentences. Do not reflow lines you are not otherwise changing.
- Internal links use Jekyll link tags: `[text]({% link setup/chrony.md %})`.
- ASCII only.

# Updating vendor support pages

When updating pages in this directory, follow
@docs/internals/vendor-support.md, especially the vendor page section.

## Notes on tags in the message file

A vendor page's low-level configuration section can have a "Notes on tags in
the message file" list. Model it on `sinognss.md`.

- Each bullet states one fact about how the receiver behaves with the tags
  that the tag names and descriptions do not reveal. Two facts make two
  bullets, even when they concern the same tags.
- State the fact plainly. When the fact is a limitation, start with the
  limitation ("The receiver can output only one kind of ..."), then say which
  tags it affects.
- Do not give advice or hints about what to do; the reader can work that out
  from the fact.
- Do not repeat what the tag names or descriptions already show.
- Do not describe how the fact was found, or the receiver's commands, command
  arguments or error replies; name tags, not commands.
- A tag keeps the meaning `configs/gpsmsg/tags.md` gives it; a note adds what
  the vendor's receiver does besides.
- Include only verified facts; leave out any clause that has not been
  verified.

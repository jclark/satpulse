# Review findings

This file consolidates all review points raised for the Septentrio message-file
reply handling, including duplicate findings and points that were considered
but not retained.

1. **Complete blockless `$R;` replies at the real prompt.**

   The guide documents `lst` replies such as `help, grc` and `lmd, grc` whose
   output follows the `$R;` line directly and ends at the real prompt, without
   a `---->` pseudo-prompt or `$-- BLOCK`. Classifying every `$R;` packet as
   `responseAckMore` left these replies pending for blocks that never arrived.

   **Disposition:** Fixed in commit `788d48a9`. A `$R;` packet ending in
   `---->` remains `responseAckMore`; one ending at the real prompt is a
   complete `responseAck`.

2. **Frame the `$TD` line inside `lstAsciiDisplay` blocks.**

   The documented final block of `lstAsciiDisplay` contains an exact `$TD`
   line. The scanner treated every body `$` as a new message boundary, so it
   discarded the `$-- BLOCK` candidate before reaching the real prompt and the
   correlator never received `responseDone`.

   The original review suggested accepting dollar-led block content generally.
   The guides document only `$TD` inside a block; `$TE` is a separate event
   transmission outside blocks. The fix therefore preserves the general `$`
   resynchronization rule and permits only an exact CRLF-delimited `$TD` line
   in a candidate beginning with `$--`.

   **Disposition:** Fixed in commit `7c5a1feb`.

3. **Preserve an observed ACK while awaiting list blocks.**

   `responseAckMore` reports `AckAck` but stores `ackWaitMore`. `Missing()`
   treats `ackWaitMore` as a missing ACK, so if the following blocks are lost
   or truncated the tool reports both `OK` and `no response received`, as well
   as the independently correct missing-data report. The same state is also
   used for `responseWait`, where the final ACK really is still missing, so
   simply excluding every `ackWaitMore` request from the missing-ACK list would
   be wrong.

   **Disposition:** Fixed. A new `ackSuccessMore` state records the positive
   ACK while keeping the request open for its data. `responseWait` continues
   to use `ackWaitMore`, so `Missing()` still reports its final ACK as missing.

4. **Do not correlate `responseDone` to a request that was never opened by
   `responseAckMore`.**

   A final `$-- BLOCK` has the same fixed Septentrio correlation key as every
   command. If a later ordinary command is also pending, `responseDone` must
   only consider the earlier request whose ACK state records that more output
   is expected.

   **Disposition:** Fixed in commit `9d57747f`. The guard in
   `gps/msgfile/correlate.go` restricts `responseDone` to the state established
   by `responseAckMore`, with a regression case in
   `gps/msgfile/sept_test.go`.

5. **Reconsider the 4096-byte reply limit.**

   One review suggested that a large `$-- BLOCK`, such as configuration-file
   output, could exceed `rMaxLength` and fail to frame.

   **Disposition:** Not retained. The limit is a deliberate robustness bound
   in the design, and oversized single blocks are explicitly out of scope.

6. **Update the design link in the pull request body.**

   The pull request body links to `plan/septentrio-msgfile.md`, while the plan
   is stored at `plan/archive/septentrio-msgfile.md` on the branch.

   **Disposition:** Not retained as a required review fix.

7. **Spell the terminator character test as `IsUpper || IsDigit`.**

   One review preferred `ascii.IsUpper(b) || ascii.IsDigit(b)` over
   `ascii.IsAlnum(b) && !ascii.IsLower(b)` for the `[A-Z0-9]` grammar.

   **Disposition:** Not retained. This is equivalent style feedback rather
   than a correctness issue.

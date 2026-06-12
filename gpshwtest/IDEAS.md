# Ideas for realizing GOAL.md

Suggested design improvements toward the requirements in `GOAL.md`. These are ideas, not decisions: if implementation experience finds something better, do that instead, and update this file.

## Plan / execute / analyze as the top-level loop

The program's job decomposes into a loop: figure out what to execute, execute it, analyze the records, repeat. Making that the explicit structure, instead of probe logic interleaved with execution:

- **Plan** takes everything known so far (nothing on first contact; prior analyses afterwards) and produces the next batch of invocations as data, each step carrying its intent in model vocabulary: what is requested and what kind of step it is (set, readback, observe-capture, restore). Adaptivity lives here and only here - signal cases generated from the discovered supported set, finer probes after a coarse quantum was found, the next discriminating experiment in save-granularity hypothesis testing.
- **Execute** is dumb and robust: run the planned invocations, record intent + result + packet log, interpret nothing. Restore steps are planned up front from the initial readback and executed unconditionally at exit, so crash-safety falls out of the structure.
- **Analyze** is a pure offline function over a set of recorded logs, answering two separate questions from the same records: are there guarantee violations, and what is the characterization. Because it never touches hardware, it re-runs over archived logs whenever the checks or the vocabulary improve, and archived runs double as its test fixtures. It should be a standalone entry point (analyze a runs directory), which is also how an unfamiliar receiver's behavior gets re-examined without hardware.

The current code maps onto this without rewrite: `tool.py` is most of execute (add the intent record); the probe tables and case generators become plan; `characterize.py` plus the inline guarantee checks become analyze; the loop driver replaces the hardwired sequence in `__main__.py`. For a routine sweep the loop degenerates to plan-once / execute / analyze.

Save-granularity discovery is the purest fit: plan a discriminating experiment among the candidate partitions (per-property, gen 8-like sections, single group - the candidates are visible in satpulse's own backends), execute the save/reload cycle, analyze the survivors, repeat until one hypothesis stands.

## Robustness for unattended runs

The receiver must end up restored even when a run dies. Ideas:

- Plan the restore tail before making any change (the initial readback determines it) and execute it unconditionally - on success, on failure, on crash.
- Make runs resumable: with the plan as data and every completed step recorded, a crashed run can be picked up at the first unexecuted step, or at worst its restore tail can be run alone.
- Treat a failed restore as loud: the next run's initial readback can verify the world matches some recorded as-found state and refuse quietly to compound the damage.

# Ideas for realizing GOAL.md

Suggested design improvements toward the requirements in `GOAL.md`. These are ideas, not decisions: if implementation experience finds something better, do that instead, and update this file.

## Plan / execute / analyze as the top-level loop

The program's job decomposes into a loop: figure out what to execute, execute it, analyze the records, repeat. Making that the explicit structure, instead of probe logic interleaved with execution:

- **Plan** takes everything known so far (nothing on first contact; prior analyses afterwards) and produces the next batch of invocations as data, each step carrying its intent in model vocabulary: what is requested and what kind of step it is (set, readback, observe-capture, restore). Adaptivity lives here and only here - signal cases generated from the discovered supported set, finer probes after a coarse quantum was found, the next discriminating experiment in save-granularity hypothesis testing.
- **Execute** is dumb and robust: run the planned invocations, record intent + result + packet log, interpret nothing. Restore steps are planned up front from the initial readback and executed unconditionally at exit, so crash-safety falls out of the structure.
- **Analyze** is a pure offline function over a set of recorded logs, answering two separate questions from the same records: are there guarantee violations, and what is the characterization. Because it never touches hardware, it re-runs over archived logs whenever the checks or the vocabulary improve, and archived runs double as its test fixtures. It should be a standalone entry point (analyze a runs directory), which is also how an unfamiliar receiver's behavior gets re-examined without hardware.

Status: the execute/analyze split is implemented. Every step is recorded with its intent in raw.jsonl (`tool.py`), the driving in `probes.py` renders no verdicts, and `analyze.py` derives failures and the characterization purely from a run directory - a live run analyzes its own records through the same path `--analyze` uses on archived ones, so there is one verdict pipeline and it is exercised on every run. Plan is not yet data: driving is still imperative (adaptive choices read invocation results directly), and restores are interleaved per probe family rather than planned up front as an unconditional tail. Moving to plan-as-data is worth it when save-granularity discovery needs hypothesis-driven experiment selection, or when resumability (below) is tackled.

Save-granularity discovery was implemented without plan-as-data: one experiment per property, with each experiment's post-reload readback threading in as the next baseline, turned out to be conclusive in a single pass (no hypothesis iteration needed). What the experiments found also broke the assumed model: the persists-together relation is not a partition when a property's realization spans NVM sections (the gen 8 timeGNSS case), so the characterization records groups when the observations form a partition and the per-property observations verbatim when they do not.

## Robustness for unattended runs

The receiver must end up restored even when a run dies. Ideas:

- Plan the restore tail before making any change (the initial readback determines it) and execute it unconditionally - on success, on failure, on crash. [Done as an emergency tail: a run that aborts in-process (tool failure, interrupt) restores everything best-effort, recorded as usual.]
- Make runs resumable: with the plan as data and every completed step recorded, a crashed run can be picked up at the first unexecuted step, or at worst its restore tail can be run alone. [The restore tail alone is done: --restore-from RUNDIR derives the tail from a crashed run's records, for deaths no in-process tail can cover (kill -9); validated with a real kill. Resume-at-first-unexecuted-step remains an idea, and would need plan-as-data.]
- Treat a failed restore as loud: the next run's initial readback can verify the world matches some recorded as-found state and refuse quietly to compound the damage. [Open; in practice a poisoned state has shown up loudly anyway, as identification or baseline failures.]

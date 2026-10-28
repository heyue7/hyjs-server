# Read-only Trial/Elite replay diagnostic

Build with the project Go toolchain for the target host. Run against a private local-to-server decoded `FrozenP6RushSeries` JSON file:

```
trial87diag series.json private-output-directory
trial87diag series.json private-output-directory original-record.pb
```

The first form writes the original typed Start, matching playback identity (fixed factors1/1), automatic preview and bounded per-round control list. The second compares an independently produced original record against automatic, raw live-controlled and actual P7 End calculations. It reads files only, never opens a database and never settles a fight. Output contains difference summaries without actor identifiers or protocol bodies. Keep production inputs and original output on the server; remove them and any temporary runtime after diagnosis. Use one process, GOMAXPROCS1 and low priority.

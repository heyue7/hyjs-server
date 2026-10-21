package constUtil

import "time"

// TerminalWorkerInterval limits polling while allowing same-day restart recovery.
const TerminalWorkerInterval = time.Minute

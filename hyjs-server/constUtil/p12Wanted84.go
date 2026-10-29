package constUtil

// P12WantedRetryableEndCode identifies a rejected recording whose paid fight
// was still pending when the rejection metadata was committed.
const P12WantedRetryableEndCode = "90005"

// P12WantedRecoveryPrefix extends the original 55003 rebelId value only for
// recovery of a specific already-paid fight. Normal instance IDs stay intact.
const P12WantedRecoveryPrefix = "wanted-recover:"

// P12WantedRecoveryCommitted is returned only after validating a committed
// paid fight and its receipt; no new attack is created by this recovery.
const P12WantedRecoveryCommitted = "本次悬赏挑战已结算"

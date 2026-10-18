# Original-login credential cache

`build.py` overlays the installed `a5ecbcd…` client. `package.py` produces only
`build/hyjs-login-cache-final-candidate.apk`; it never promotes current or rollback.
The original registration, modern IME, announcement, tutorial and wallet remain.

The current login panel shows `记住密码` only. Automatic login is hidden and
disabled, including legacy encrypted preferences whose `auto` field was true.
`boot` and `ready` preserve the managed bridge ABI but never submit login;
showing the form always waits for the user's login button or IME action.
Remembered credentials still prefill. Android system autofill remains enabled.
Only an accepted successful original `SCUserLogin` response commits credentials;
unchecking remember clears that encrypted cache.

Storage uses Android Keystore AES/GCM (`hyjs.remember-login.v1`) and the private
`hyjs-encrypted-login-v1` SharedPreferences file, containing only `ciphertext` and
`iv`. Key/decrypt/write failure clears unusable cache and returns to manual login.
No session is persisted. The original `userName` / `passWord` PlayerPrefs keys
are deleted individually on login initialization; their four original accessors
now use Java process-memory strings. No old plaintext credentials are imported.
The existing stripped original authCode log remains stripped.

Each original network delegate is bound to a fresh `MemberwiseClone` snapshot
of the original login state with an immutable RuntimeHelpers identity. Java rejects zero and every identity already seen in the process, so collisions and GC identity reuse cannot authenticate a later request. The network delegate retains its snapshot while pending. The original callback's
only instance reads are its shared `manager`; shared writes are
`playerData.playerId` and `manager.isFirstCreate`, with other state in StaticData.
It has no callback-local state writes or identity comparisons. A guard rejects
cancelled, timed-out, duplicate and previous-attempt callbacks before any state
write. The original state machine and original protocol handler remain in use.

Final static proof: `analysis/client-login-cache-20261018/overlay-r7/manifest.json`.
The first verification-selector failure is retained separately. A failed phone
loader candidate introduced FieldPtr by adding three managed fields; final r6
adds no fields, methods or types and keeps FieldPtr absent. The original IL was
reviewed again after this compatibility repair.
APK proof: `analysis/client-login-cache-20261018/apk-build-r4.json`.
Original 547 battle engine bodies and 10,288 unrelated ZIP entries stay exact.
Phone acceptance is recorded separately and must not be inferred from this file.

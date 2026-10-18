# Original-login credential cache

`build.py` overlays the installed `a5ecbcd…` client. `package.py` produces only
`build/hyjs-login-cache-final-candidate.apk`; it never promotes current or rollback.
The original registration, modern IME, announcement, tutorial and wallet remain.

The original login panel gains two small native checkbox labels, initially off:
`记住密码` and `自动登录`. Auto implies remember. Unchecking remember immediately
clears the encrypted cache and auto option. Only an accepted successful original
`SCUserLogin` response commits credentials. Registration success, old prefs,
draft input and failed logins cannot seed automatic login. The original character
creation path runs after original login as before.

Storage uses Android Keystore AES/GCM (`hyjs.remember-login.v1`) and the private
`hyjs-encrypted-login-v1` SharedPreferences file, containing only `ciphertext` and
`iv`. Key/decrypt/write failure clears unusable cache and returns to manual login.
No session is persisted. The original `userName` / `passWord` PlayerPrefs keys
are deleted individually on login initialization; their four original accessors
now use Java process-memory strings. No old plaintext credentials are imported.
The existing stripped original authCode log remains stripped.

Cold start consumes auto at most once. Explicit account switching or returning
to login suppresses automatic login for the running process while preserving an
explicitly remembered encrypted credential for the next cold start. Failures and
20-second timeout return to the original manual input and reset `isLogining`.

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

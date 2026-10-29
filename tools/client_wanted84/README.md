# Wanted 84 recovery overlay

The pinned 1.0.80 assembly contains the feedback65 session-local Start recovery helper. Its failure callback previously repeated the battle on every End failure. An unknown or lost reply can instead be an already committed End, so a replay with different controls correctly conflicts with the original receipt.

This overlay captures the original 55011 request and its callback. A Wanted-specific `90005` error permits the same paid Start to be replayed only after the server classified the recorded input as rejected and rechecked that the fight was still pending. All other failures retain the exact End for retry. On opening the original Wanted page, one bounded retry waits for the page's actual list response and the following SCPlayer snapshot; it still works when the old boss is no longer listed. A real accepted receipt triggers a fresh original list/SCPlayer query. It does not add the End's resource deltas to a snapshot that may already include them. If a rejected fight's boss is absent from the successful original list, the session-local retry state is retired after the following player snapshot; the server retains its paid fight and evidence.

The cache is process-local and bound to the original player object, player ID, session, fight, page and callback generation. The original `BackToLogin` calls `StaticData.ClearAllData`, which creates a new player object; older callbacks cannot apply their data to that login. No durable client recovery is claimed after process termination.

The four pinned Main methods preserve their original instructions, branches, locals, other bodies and all 547 original battle-engine methods. The main connection's original timeout box retains and resends its original network packet; the overlay does not intercept that timeout. A `90005` replay uses the original 55003 request with `rebelId` set to `wanted-recover:<fightId>`, and the server verifies that named paid fight and reacquires its lease. After reacquisition, a late failed End cannot release that lease or overwrite the earlier rejection; further failures retain the same paid fight until it settles or the boss expires or is defeated. This value cannot create a new fight if an overlapping End committed. Ordinary instance IDs retain their existing semantics. The helper's existing public ABI and non-Wanted selection source are checked. Build and test in the agreed compilation slot:

```sh
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_wanted84/run_tests.py
rtk proxy /Users/lirui/LR/hyjs/.analysis-venv/bin/python tools/client_wanted84/build.py --base-apk /Users/lirui/LR/hyjs/build/黑衣剑士灵魂之花v1.0.80.apk --output-directory tools/client_wanted84/overlay-80
```

For combined delivery, add `--input-managed <composed-Assembly-CSharp.dll> --input-managed-sha256 <sha256>` and use a fresh output directory. The four target bodies remain individually pinned; every other method and all 547 engine methods are checked against that exact composed input. The base APK supplies the previously released helper and dependencies only.

The tests use bounded callback doubles; they do not prove Unity timing, a real MySQL transaction, or native device behavior. Package composition, server error mapping, 55001/SCPlayer ordering, actual loss/retry, paid retry, concurrency and cold reconnect require integrated verification by the controlling session. There is no production login, installation, deployment or package operation here.

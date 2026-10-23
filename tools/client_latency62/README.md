# latency62 managed overlay

Fixed input: build/hyjs-recycle61-r3.apk SHA256 65a9b29bbf176a2788abf7896116b14794c7a818eb673b8ad71a0084ef8dcc3e.

Only NetMaskBoxController.Init/Update, ShortWebClient.GetWebRequest and BoxManager.CreateNetTimeOut change. Original input MaskObject, Show/Clean, joystick, transport serialization, queue, result callbacks, timeout/error handlers, retry and engine remain exact raw methods; timeout creation retains original instructions with one read-only-to-business front-layer hook.

At 250ms, the original loading_xiaoquan texture (66x66) fades in over 100ms at original mask center; rotates 150 degrees/s using realtime. At 1.5s it crossfades over 100ms into original full Ani. No minimum duration; parent disable immediately cancels visuals and resets alpha. The original loading_fangtiao_01 texture bakes the network text, so full Ani stays hidden during compact feedback. See analysis/latency62-20261024/client-texture-reference.png and client-audit.md for source evidence. Original texture files are inherited unchanged.

Original HTTP request receives KeepAlive=true and ServicePoint.Expect100Continue=false / UseNagleAlgorithm=false. Original URI, proxy=null, 15s timeout/read timeout remain. ServicePoint flags apply to the matching endpoint's existing ServicePoint; no global ServicePointManager defaults change.

Run via shared .analysis-venv/bin/python tools/client_latency62/build.py --output-directory build/latency62-formal-r4. Build output is managed overlay only. Policy/WebRequest host probe does not prove Unity/Mono runtime rendering, duplicate tap behavior or measured network speed; these need fixed-candidate phone/controlled latency QA by the main agent.

Packaging contract: build derives inherited Java hashes, classes2 input SHA and class hierarchy checks directly from fixed base. Managed-only payload excludes every Java entry, preserving its entire public/protected ABI by exact byte inheritance. The build calls the actual package_native.main entry with a write-blocking ZIP wrapper and stops at its first unsigned APK ZIP creation after all pre-write source/payload/Java/managed ABI/547-engine/signature-certificate checks. Post-write compressed-entry verification, signing and alignment remain the main packager responsibility.

R3 corrects compact local Z to 0, matching original Ani/loading_donghua/GameObject16/Plane14 (loading_xiaoquan) visual chain. Their relative Z values are all 0; root is positioned at -500 by BoxManager. MaskObject Z=-700 and bg Z=-720 belong to separate blocking/background planes and are unsuitable anchors for the ring. Compact remains a root child of the original UIPanel with centered X/Y. Only FeedbackView.CreateCompact implementation changes from r2.

R4 raises each network mask once at Init and each timeout box once after original creation. FrontLayer snapshots own panel depth/sortingOrder, scans other active UIPanel.list once, shifts all own panels by equal depth/order deltas above external maxima, and restores on disable/destroy. This preserves own relative order, original colliders/buttons and callbacks. No per-frame global scans. UIPanel.depth setter sorts the original list; sortingOrder setter calls UpdateDrawCalls, which applies UIDrawCall.sortingOrder. Original NGUI therefore controls rendering and hit-depth through its own panel properties.

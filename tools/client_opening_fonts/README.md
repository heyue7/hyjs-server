# Opening activity label fonts

Changes exactly three original UILabel font references from Numbers190 to Words191 already present in the same APK: two opening-activity labels and the TownPage coin value label. Words includes 万/亿, so the original coin formatter can render amounts such as 893万 without relying on platform fallback. It keeps all-days-open behavior, amount formatting, strings, font size, layout, colors and activity state intact.

```bash
rtk proxy .analysis-venv/bin/python tools/client_opening_fonts/build.py --output-directory analysis/playability31-20261020/client/fonts-r2-coin
```

Requires a fresh output directory. The APK is pinned by SHA256. Each entry and target object is separately pinned by SHA256, pathID, byte location, object size, UILabel script, parent field binding and original Font PPtr. Each pin carries its original external fileID (2 for the activity labels, 8 for TownPage). No Unity asset serialization is performed: one pathID byte changes in each resource. The verifier reloads all three files, compares all object bytes and metadata, and checks embedded Font cmap coverage including 万/亿.

Outputs only three replacement resource entries plus manifest. No APK packaging, DLL patch, font-asset replacement, deployment or device operations occur. Root handles integration and device verification. The historical font-only package bc502f0e and its two-label manifest remain valid records of that earlier scope; this tool does not rewrite them.

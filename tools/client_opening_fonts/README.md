# Opening activity label fonts

Changes exactly two original UILabel font references from Numbers190 to Words191 already present in the same APK. It keeps all-days-open behavior, strings, font size, layout, colors and activity state intact.

```bash
rtk proxy .analysis-venv/bin/python tools/client_opening_fonts/build.py --output-directory analysis/playability31-20261020/client/opening-fonts-r1
```

Requires a fresh output directory. The APK is pinned by SHA256. Each entry and target object is separately pinned by SHA256, pathID, byte location, object size, UILabel script, parent field binding and original Font PPtr. No Unity asset serialization is performed: one pathID byte changes in each resource. The verifier reloads both files, compares all object bytes and metadata, and checks embedded Font cmap coverage.

Outputs only two replacement resource entries plus manifest. No APK packaging, DLL patch, font-asset replacement, deployment or device operations occur. Root handles integration and device verification.

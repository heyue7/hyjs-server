"""Four immutable generic growth probes; no predictions or authority promotion."""
from pathlib import Path
import json
import shutil
from case_sets import ROOT, sha

SOURCE = ROOT / 'analysis/p6/generic-four-source-20261017/manifest.json'
SOURCE_SHA = 'e9e999cffb68071aced70168e48a8a4ccc9fa3c81585add6b3017698cd5802f5'


def prepare(work):
    if sha(SOURCE) != SOURCE_SHA:
        raise ValueError('Fixed generic four source changed')
    source = json.loads(SOURCE.read_bytes())
    if len(source['cases']) != 4 or source['nativeCompared'] or source['authoritative']:
        raise ValueError('Four diagnostic source rows only')
    if sha(ROOT / source['sourceManifest']) != source['sourceManifestSHA256']:
        raise ValueError('Original Go export source changed')
    folder = work / 'cases/generic-mechanism-four/highest'
    folder.mkdir(parents=True)
    rows = []
    for row in source['cases']:
        wire = SOURCE.parent / row['wire']
        if Path(row['wire']).name != row['wire'] or sha(wire) != row['wireSHA256'] or row['battleType'] != 11 or row['nativeScope'] != 'original-pvp-automatic-ai':
            raise ValueError('Pinned original automatic source changed')
        shutil.copy2(wire, folder / wire.name)
        rows.append({**row, 'sourceWire': str(wire.relative_to(ROOT)), 'localWire': str((folder / wire.name).relative_to(work))})
    return [{'set': 'generic-mechanism-four', 'caseDirectory': str(folder.relative_to(work)),
             'sourceManifest': str(SOURCE.relative_to(ROOT)), 'sourceManifestSHA256': SOURCE_SHA,
             'phoneAnchor': None, 'cases': rows}]

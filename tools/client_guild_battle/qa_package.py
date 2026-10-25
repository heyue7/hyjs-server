"""Compose the guild candidate's four original network methods for isolated QA."""
import hashlib
import json
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.client_recycle_selection import qa_package as inherited

ORIGIN = 'http://127.0.0.1:28993'

if __name__ == '__main__':
    # Reuse its exact four-method/547 engine/signature/ZIP verifier. The complete
    # guild overlay remains inherited bytes from the explicitly pinned input.
    inherited.ORIGIN = ORIGIN
    inherited.main()
    record = Path(sys.argv[sys.argv.index('--record') + 1]).resolve()
    data = json.loads(record.read_text())
    assert data['qaOrigin'] == ORIGIN
    data.update(qaOnly=True, neverPromote=True,
                composition='guild candidate inherited; four original origins and wallet config only')
    data['sourcePins'][str(Path(__file__).relative_to(ROOT))] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    record.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')

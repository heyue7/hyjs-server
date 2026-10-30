"""Run the 85 line and fragment transactions with the existing managed runner."""
from pathlib import Path
import sys

import validate_feedback84_mysql as runner

ROOT = Path(__file__).resolve().parents[1]
runner.BINARY = ROOT / 'build/fragment-box85/service.test'
runner.OUTPUT_ROOT = ROOT / 'analysis/fragment-box85/mysql'
runner.CASES = [
 ('boss-lines', 'p1', 'TestFieldBossLines85LegacyPrimaryKeyMigration'),
 ('boss-lines', 'p1', 'TestFieldBossLines85PresenceFallback'),
 ('boss-lines', 'p1', 'TestFieldBossLines85RealFightColdAndIndependentRespawn'),
 ('fragment-boxes', 'p1', 'TestFragmentBoxes85BatchChoicesAndReceipt'),
]
if __name__ == '__main__':
 sys.exit(runner.main())

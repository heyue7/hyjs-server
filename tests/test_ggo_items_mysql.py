"""Run GGO transactions and real JSON handlers on owned managed MySQL schemas."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from hyjs_managed_mysql import ManagedMySQL

ROOT = Path(__file__).resolve().parents[1]


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--case", choices=["conversion", "authority", "concurrency", "http"])
    selected = parser.parse_args().case
    pool = ManagedMySQL()
    started = time.monotonic()
    report = {"status": "RUNNING", "scope": "isolated MySQL GGO conversion, purchase concurrency and real handler httptest", "cases": []}
    destination = ROOT / ('analysis/ggo-items-20261023/mysql' + ('-' + selected if selected else '') + '.json')
    try:
        cases = [
            ('conversion', './service', '^TestGGOConversionNoHitInvalidAndAtomicRollback$'),
            ('authority', './service', '^TestGGOShopAuthorityReceiptAndPriceEdit$'),
            ('concurrency', './service', '^TestGGOMySQLConcurrentTransactions$'),
            ('http', './handler', '^TestGGOShopMySQLHTTP$'),
        ]
        for index, (label, package, selector) in enumerate(cases):
            if selected and label != selected:
                continue
            connection = pool.allocate('p1_ggo_' + str(index))
            environment = dict(os.environ, HYJS_P1_MYSQL_HOST=connection.host,
                               HYJS_P1_MYSQL_USER=connection.user,
                               HYJS_P1_MYSQL_PASSWORD=connection.password,
                               HYJS_P1_MYSQL_DATABASE=connection.database)
            run = subprocess.run(['rtk', 'proxy', 'go', 'test', '-p', '1', package,
                                  '-run', selector, '-count=1', '-v'],
                                 cwd=ROOT / 'hyjs-server', env=environment,
                                 text=True, capture_output=True, timeout=300)
            output = pool.redact(run.stdout + run.stderr)
            print(label + ':\n' + output[-6000:])
            case = {'case': label, 'status': 'PASS' if run.returncode == 0 else 'FAIL',
                    'exitCode': run.returncode, 'database': connection.database}
            if run.returncode == 0:
                case['transactionalTables'] = pool.sql(connection.database,
                    "SELECT TABLE_NAME, ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME IN ('profiles','ggo_shop_receipts','account_wallet_balances','account_wallet_entries','terminal_incomes','shop_states','p4_commerce_stocks') ORDER BY TABLE_NAME;").strip().splitlines()
            report['cases'].append(case)
            pool.release(connection.database)
            if run.returncode:
                report['status'] = 'FAIL'
                break
        else:
            report['status'] = 'PASS'
    finally:
        pool.close()
        report.update(ownedDatabaseAndAccountReleased=True, seconds=round(time.monotonic() - started, 3))
        destination.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    return 0 if report['status'] == 'PASS' else 1


if __name__ == '__main__':
    sys.exit(main())

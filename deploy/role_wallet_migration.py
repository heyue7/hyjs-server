"""Read-only gates for the one-time account-to-role wallet cutover.

The application owns the migration transaction. Deployment stops legacy writers,
backs up the database, and uses these queries to verify immutable source history
and every copied field without retaining business rows in deployment evidence.
"""
import hashlib

SOURCES = {
    'account_wallet_balances': ['account_id', 'balance_cents', 'revision'],
    'account_wallet_entries': ['account_id', 'key', 'server_id', 'player_id', 'legacy_key', 'kind', 'delta_cents', 'balance_after', 'revision', 'order_id', 'at', 'source', 'reason_id', 'label'],
    'terminal_account_states': ['account_id', 'seen_revision', 'delivered_revision'],
    'terminal_incomes': ['account_id', 'id', 'label', 'amount_cents', 'created_at', 'revision', 'source'],
    'terminal_bursts': ['account_id', 'request_id', 'day', 'award_points'],
}

EXPECTED = """SELECT account_id FROM account_wallet_balances WHERE balance_cents<>0 OR revision<>0
UNION SELECT account_id FROM account_wallet_entries
UNION SELECT account_id FROM terminal_account_states
UNION SELECT account_id FROM terminal_incomes
UNION SELECT account_id FROM terminal_bursts"""


def snapshot(mysql):
    result = {}
    for table, columns in SOURCES.items():
        fields = ','.join('`' + c + '`' for c in columns)
        rows = mysql('SELECT SHA2(JSON_ARRAY(' + fields + '),256) AS digest FROM ' + table + ' ORDER BY digest;')
        result[table] = {'rows': len(rows.splitlines()), 'sha256': hashlib.sha256(rows.encode()).hexdigest()}
    return result


def before(mysql):
    conflicts = int(mysql('SELECT COUNT(*) FROM (' + EXPECTED + ') a LEFT JOIN (SELECT account_id,COUNT(*) n FROM game_account_roles GROUP BY account_id) r ON r.account_id=a.account_id WHERE COALESCE(r.n,0)<>1;').strip())
    assert conflicts == 0, 'Legacy wallet owners must have exactly one realm identity'
    exists = int(mysql("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='role_wallet_migrations';").strip())
    if exists:
        assert int(mysql('SELECT COUNT(*) FROM role_wallet_migrations;').strip()) == 0, 'One-time cutover already ran; use the regular deployment path'
    return {'legacySources': snapshot(mysql), 'expectedMigrations': int(mysql('SELECT COUNT(*) FROM (' + EXPECTED + ') a;').strip())}


def after(mysql, baseline):
    assert snapshot(mysql) == baseline['legacySources'], 'Legacy wallet history changed during cutover'
    count = int(mysql('SELECT COUNT(*) FROM role_wallet_migrations;').strip())
    assert count == baseline['expectedMigrations'], 'Migration marker count differs'
    missing = int(mysql('SELECT COUNT(*) FROM (' + EXPECTED + ') a LEFT JOIN role_wallet_migrations m ON m.account_id=a.account_id WHERE m.account_id IS NULL;').strip())
    assert missing == 0, 'A legacy wallet was not migrated'
    copied = {}
    for source, target, identity in [
        ('account_wallet_entries', 'role_wallet_entries', ['key']),
        ('terminal_account_states', 'terminal_role_states', []),
        ('terminal_incomes', 'terminal_role_incomes', ['id']),
        ('terminal_bursts', 'terminal_role_bursts', ['request_id']),
    ]:
        # Destination realm/player belong to the unique role, while every other
        # historical field (including SQL NULL order IDs) must remain exact.
        fields = [c for c in SOURCES[source] if c not in ('server_id', 'player_id')]
        same = ' AND '.join('(s.`' + c + '` <=> d.`' + c + '`)' for c in fields)
        if source == 'terminal_account_states':
            # Existing clients may legitimately acknowledge/poll immediately
            # after startup. The old snapshot stays exact; new cursors may only
            # advance and must remain bounded by actual role income history.
            same = ('s.account_id=d.account_id AND d.seen_revision>=s.seen_revision '
                    'AND d.delivered_revision>=s.delivered_revision '
                    'AND GREATEST(d.seen_revision,d.delivered_revision)<=GREATEST(s.seen_revision,s.delivered_revision,'
                    'COALESCE((SELECT MAX(i.revision) FROM terminal_role_incomes i WHERE i.server_id=d.server_id AND i.player_id=d.player_id),0))')
        keys = ''.join(' AND d.`' + c + '`=s.`' + c + '`' for c in identity)
        query = ('SELECT COUNT(*) FROM ' + source + ' s JOIN role_wallet_migrations m ON m.account_id=s.account_id '
                 'LEFT JOIN ' + target + ' d ON d.server_id=m.server_id AND d.player_id=m.player_id AND d.account_id=m.account_id' + keys +
                 ' WHERE d.account_id IS NULL OR NOT (' + same + ');')
        assert int(mysql(query).strip()) == 0, 'Historical fields differ: ' + source
        copied[source] = baseline['legacySources'][source]['rows']
    mismatch = mysql('''SELECT COUNT(*) FROM role_wallet_migrations m
LEFT JOIN account_wallet_balances old ON old.account_id=m.account_id
LEFT JOIN role_wallet_balances b ON b.server_id=m.server_id AND b.player_id=m.player_id AND b.account_id=m.account_id
WHERE m.balance_cents<>COALESCE(old.balance_cents,0) OR m.revision<>COALESCE(old.revision,0)
OR b.account_id IS NULL
OR b.balance_cents<>m.balance_cents+COALESCE((SELECT SUM(e.delta_cents) FROM role_wallet_entries e WHERE e.server_id=m.server_id AND e.player_id=m.player_id AND e.revision>m.revision),0)
OR b.revision<>GREATEST(m.revision,COALESCE((SELECT MAX(e.revision) FROM role_wallet_entries e WHERE e.server_id=m.server_id AND e.player_id=m.player_id),0))
OR m.entry_count<>(SELECT COUNT(*) FROM account_wallet_entries e WHERE e.account_id=m.account_id);''')
    assert int(mismatch.strip()) == 0, 'Role wallet balance/revision/history reconciliation failed'
    return {'migratedAccounts': count, 'legacySourcesUnchanged': True, 'historicalFieldsVerified': copied,
            'newCursorsMonotonic': True, 'balanceAndRevisionReconciled': True}

"""Install a daily-only clock in an owned, isolated native test source copy.

The canonical main is retained in its shared manifest. This file never builds,
starts an app or opens SQL. Call it only on the next coordinated native source
copy and record the resulting main SHA separately from the canonical SHA.
"""
import argparse
import hashlib
import json
from pathlib import Path


def prepare(main):
    source = main.read_text()
    assert 'HYJS_TEST_LIMIT_DAILY_CLOCK' not in source, 'clock already installed'
    anchor = '\tdefer cancel()\n'
    assert source.count(anchor) == 1
    assert '"hyjs-server/constUtil"' not in source
    assert '"strings"' not in source
    before = hashlib.sha256(main.read_bytes()).hexdigest()
    source = source.replace('"hyjs-server/config"', '"hyjs-server/config"\n\t"hyjs-server/constUtil"')
    source = source.replace('"strconv"', '"strconv"\n\t"strings"')
    source = source.replace(anchor, anchor+'''	// Test-only managed namespace guard keeps actor, P6 and entry clocks real.
	if clockPath := os.Getenv("HYJS_TEST_LIMIT_DAILY_CLOCK"); clockPath != "" {
		if cfg.Driver != "mysql" || !strings.HasPrefix(config.AppConfigIns.MysqlConfig.DbName, "hyjs_test_p11_limit_daily_") {
			return errors.New("daily cutoff test clock requires its managed test schema")
		}
		ctx = context.WithValue(ctx, constUtil.P11LimitDailyClockContextKey{}, func() time.Time {
			raw, readErr := os.ReadFile(clockPath)
			if readErr != nil {
				panic(readErr)
			}
			value, parseErr := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
			if parseErr != nil || value < 0 {
				panic("invalid managed daily cutoff clock")
			}
			if value == 0 {
				return time.Now()
			}
			return time.UnixMilli(value)
		})
	}
''')
    main.write_text(source)
    return dict(canonicalMainSHA256=before, testMainSHA256=hashlib.sha256(main.read_bytes()).hexdigest(),
                scope='Only P11LimitDailyClockContextKey, managed hyjs_test_p11_limit_daily_ schema, clock0 real wall time; no actor/P6/entry clock changes.')


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('main', type=Path)
    args = parser.parse_args()
    assert args.main.name == 'main.go'
    assert 'build/candidates' in str(args.main.resolve())
    print(json.dumps(prepare(args.main), indent=2))

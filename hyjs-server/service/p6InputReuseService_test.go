package service

import (
	"context"
	"testing"

	"hyjs-server/db"
)

func TestP6InputReuseScope(t *testing.T) {
	frozen, raw, stored := p6ReplayTableHashFixture(t)
	record := p6ReplayTableHashRecord(frozen, raw, stored)

	t.Run("scope and public decoder isolation", func(t *testing.T) {
		ctx := withP6InputReuseScope(context.Background())
		first, err := decodeP6RecordScoped(ctx, &record)
		if err != nil {
			t.Fatal(err)
		}
		nested := withP6InputReuseScope(ctx)
		second, err := decodeP6RecordScoped(nested, &record)
		if err != nil || second != first {
			t.Fatalf("nested scope did not reuse input: %v", err)
		}
		copiedRecord := record
		copiedRecord.Input = append([]byte(nil), record.Input...)
		second, err = decodeP6RecordScoped(ctx, &copiedRecord)
		if err != nil || second != first {
			t.Fatalf("equal input bytes were not reused: %v", err)
		}
		other, err := decodeP6RecordScoped(withP6InputReuseScope(context.Background()), &record)
		if err != nil || other == first {
			t.Fatalf("separate scope reused input: %v", err)
		}
		unscoped, err := decodeP6RecordScoped(context.Background(), &record)
		if err != nil || unscoped == first || unscoped == other {
			t.Fatalf("unscoped read reused input: %v", err)
		}
		nilContext, err := decodeP6RecordScoped(nil, &record)
		if err != nil || nilContext == first || nilContext == unscoped {
			t.Fatalf("nil context did not use the original decoder: %v", err)
		}
		public, err := decodeP6Record(&record)
		if err != nil || public == first || public == other || public == unscoped {
			t.Fatalf("public decoder reused input: %v", err)
		}
		if len(public.Tables["Hero"]) == 0 || len(first.Tables["Hero"]) == 0 {
			t.Fatal("fixture has no Hero table")
		}
		originalID := first.Tables["Hero"][0]["id"]
		public.Tables["Hero"][0]["id"] = "public-only"
		if first.Tables["Hero"][0]["id"] != originalID || other.Tables["Hero"][0]["id"] != originalID {
			t.Fatal("public decoder shared mutable table rows with a scope")
		}
	})

	changes := []struct {
		name   string
		change func(*db.P6BattleRecord)
	}{
		{"ID", func(r *db.P6BattleRecord) { r.ID += "-other" }},
		{"PlayerID", func(r *db.P6BattleRecord) { r.PlayerID++ }},
		{"Seed", func(r *db.P6BattleRecord) { r.Seed++ }},
		{"InputHash", func(r *db.P6BattleRecord) { r.InputHash += "-other" }},
		{"ConfigurationHash", func(r *db.P6BattleRecord) { r.ConfigurationHash += "-other" }},
		{"EngineVersion", func(r *db.P6BattleRecord) { r.EngineVersion += "-other" }},
		{"ClientVersion", func(r *db.P6BattleRecord) { r.ClientVersion += "-other" }},
		{"stored input bytes", func(r *db.P6BattleRecord) { r.Input = append([]byte(nil), raw...) }},
	}
	for _, tc := range changes {
		t.Run(tc.name, func(t *testing.T) {
			ctx := withP6InputReuseScope(context.Background())
			first, err := decodeP6RecordScoped(ctx, &record)
			if err != nil {
				t.Fatal(err)
			}
			changed := record
			tc.change(&changed)
			want, wantErr := decodeP6Record(&changed)
			got, gotErr := decodeP6RecordScoped(ctx, &changed)
			if wantErr != nil {
				if gotErr == nil || gotErr.Error() != wantErr.Error() {
					t.Fatalf("changed input error = %v, original decoder error = %v", gotErr, wantErr)
				}
				return
			}
			if gotErr != nil || got == nil || want == nil || got == first {
				t.Fatalf("changed identity or stored bytes reused the entry: %v", gotErr)
			}
		})
	}

	t.Run("stored bytes copied and failed reads not cached", func(t *testing.T) {
		ctx := withP6InputReuseScope(context.Background())
		mutable := record
		mutable.Input = append([]byte(nil), record.Input...)
		first, err := decodeP6RecordScoped(ctx, &mutable)
		if err != nil {
			t.Fatal(err)
		}
		mutable.Input[len(mutable.Input)-1] ^= 1
		_, wantErr := decodeP6Record(&mutable)
		if _, err = decodeP6RecordScoped(ctx, &mutable); wantErr == nil || err == nil || err.Error() != wantErr.Error() {
			t.Fatalf("mutated stored bytes error = %v, original decoder error = %v", err, wantErr)
		}
		mutable.Input[len(mutable.Input)-1] ^= 1
		again, err := decodeP6RecordScoped(ctx, &mutable)
		if err != nil || again != first {
			t.Fatalf("failed read changed the last successful entry: %v", err)
		}
		_, wantErr = decodeP6Record(nil)
		if _, err = decodeP6RecordScoped(ctx, nil); wantErr == nil || err == nil || err.Error() != wantErr.Error() {
			t.Fatalf("nil record error = %v, original decoder error = %v", err, wantErr)
		}

		empty := withP6InputReuseScope(context.Background())
		bad := record
		bad.InputHash += "-invalid"
		if _, err = decodeP6RecordScoped(empty, &bad); err == nil {
			t.Fatal("invalid record was accepted")
		}
		if empty.Value(p6InputReuseScopeKey{}).(*p6InputReuseScope).last != nil {
			t.Fatal("failed decode populated the scope")
		}
		if _, err = decodeP6RecordScoped(empty, &record); err != nil {
			t.Fatalf("valid record failed after rejected decode: %v", err)
		}
	})

	t.Run("only last input retained", func(t *testing.T) {
		ctx := withP6InputReuseScope(context.Background())
		first, err := decodeP6RecordScoped(ctx, &record)
		if err != nil {
			t.Fatal(err)
		}
		otherRecord := record
		otherRecord.ID += "-other"
		other, err := decodeP6RecordScoped(ctx, &otherRecord)
		if err != nil || other == first {
			t.Fatalf("second record did not replace first: %v", err)
		}
		again, err := decodeP6RecordScoped(ctx, &otherRecord)
		if err != nil || again != other {
			t.Fatalf("second record was not reused: %v", err)
		}
		firstAgain, err := decodeP6RecordScoped(ctx, &record)
		if err != nil || firstAgain == first || firstAgain == other {
			t.Fatalf("evicted first record was reused: %v", err)
		}
	})
}

func BenchmarkP6InputReuseFiveReads(b *testing.B) {
	frozen, raw, stored := p6ReplayTableHashFixture(b)
	record := p6ReplayTableHashRecord(frozen, raw, stored)
	b.Run("original_decoder", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(stored)) * 5)
		for i := 0; i < b.N; i++ {
			for n := 0; n < 5; n++ {
				if _, err := decodeP6Record(&record); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("request_scope", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(stored)) * 5)
		for i := 0; i < b.N; i++ {
			ctx := withP6InputReuseScope(context.Background())
			for n := 0; n < 5; n++ {
				if _, err := decodeP6RecordScoped(ctx, &record); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
}

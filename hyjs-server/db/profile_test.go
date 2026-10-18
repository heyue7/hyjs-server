package db

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestInitGameMigratesPythonProfilesWithoutChangingSavedData(t *testing.T) {
	for _, passwords := range []bool{false, true} {
		t.Run(map[bool]string{false: "early-schema", true: "password-schema"}[passwords], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "python.sqlite3")
			old, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Close()
			// 直接建立 Python Store 的历史表，保留 NULL 与原始 JSON 字符串进行迁移对照。
			if _, err = old.Exec(`CREATE TABLE profiles (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				uid TEXT NOT NULL UNIQUE,
				session BLOB NOT NULL,
				player TEXT,
				created_at INTEGER
			)`); err != nil {
				t.Fatal(err)
			}
			if passwords {
				for _, name := range []string{"password_salt", "password_hash"} {
					if _, err = old.Exec("ALTER TABLE profiles ADD COLUMN " + name + " BLOB"); err != nil {
						t.Fatal(err)
					}
				}
				if _, err = old.Exec("ALTER TABLE profiles ADD COLUMN game TEXT"); err != nil {
					t.Fatal(err)
				}
			}
			var (
				session = bytes.Repeat([]byte{7}, 16)
				player  = `{"playerId":7,"name":"迁移存档","coin":760}`
				game    = `{"run":{"fight":{"fight_id":"old-fight"}}}`
				salt    = bytes.Repeat([]byte{8}, 16)
				digest  = bytes.Repeat([]byte{9}, 32)
			)
			if _, err = old.Exec("INSERT INTO profiles(id,uid,session,player,created_at) VALUES (?,?,?,?,?)", 7, "python-player", session, player, 1700000000000); err != nil {
				t.Fatal(err)
			}
			if passwords {
				if _, err = old.Exec("UPDATE profiles SET game=?,password_salt=?,password_hash=? WHERE id=7", game, salt, digest); err != nil {
					t.Fatal(err)
				}
			}
			// 已删除账号也占用过 ID；迁移不能让新账号重新取得这个编号。
			if _, err = old.Exec("INSERT INTO profiles(id,uid,session) VALUES (99,'deleted-account',?)", session); err != nil {
				t.Fatal(err)
			}
			if _, err = old.Exec("DELETE FROM profiles WHERE id=99"); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				if err = InitGame(context.Background(), path); err != nil {
					t.Fatal(err)
				}
				current := GetIns()
				conn, connErr := current.DB()
				if connErr != nil {
					t.Fatal(connErr)
				}
				var row Profile
				if err = current.First(&row, 7).Error; err != nil {
					conn.Close()
					t.Fatal(err)
				}
				if row.Uid != "python-player" || !bytes.Equal(row.Session, session) || row.Player == nil || *row.Player != player || row.CreatedAt == nil || *row.CreatedAt != 1700000000000 {
					conn.Close()
					t.Fatal("migration changed the existing profile")
				}
				if passwords {
					if row.Game == nil || *row.Game != game || !bytes.Equal(row.PasswordSalt, salt) || !bytes.Equal(row.PasswordHash, digest) {
						conn.Close()
						t.Fatalf("migration changed existing fields: gamePresent=%t gameEqual=%t saltEqual=%t hashEqual=%t", row.Game != nil, row.Game != nil && *row.Game == game, bytes.Equal(row.PasswordSalt, salt), bytes.Equal(row.PasswordHash, digest))
					}
				} else if row.Game != nil || row.PasswordHash != nil || row.PasswordSalt != nil {
					conn.Close()
					t.Fatal("migration filled columns that must remain NULL")
				}
				conn.Close()
			}
			if err = InitGame(context.Background(), path); err != nil {
				t.Fatal(err)
			}
			current := GetIns()
			conn, err := current.DB()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			fresh := &Profile{Uid: "new-account", Session: session}
			if err = current.Create(fresh).Error; err != nil {
				t.Fatal(err)
			}
			if fresh.Id <= 99 {
				t.Fatalf("migration reused a previous account ID: %d", fresh.Id)
			}
			if err = current.Create(&Profile{Uid: "python-player", Session: session}).Error; err == nil {
				t.Fatal("migration lost account uniqueness")
			}
		})
	}
}

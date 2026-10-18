"""Local development profiles. This does not authenticate original SDK accounts."""
import hmac
import hashlib
from contextlib import contextmanager
import json
import math
from pathlib import Path
import secrets
import sqlite3
import time


class Store:
    def __init__(self, path):
        self.path = str(path)
        Path(self.path).parent.mkdir(parents=True, exist_ok=True)
        with self.connect() as db:
            db.execute("""CREATE TABLE IF NOT EXISTS profiles (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                uid TEXT NOT NULL UNIQUE,
                session BLOB NOT NULL,
                player TEXT,
                created_at INTEGER
            )""")
            columns = {r[1] for r in db.execute('PRAGMA table_info(profiles)')}
            for column in ('password_salt', 'password_hash'):
                if column not in columns:
                    db.execute(f'ALTER TABLE profiles ADD COLUMN {column} BLOB')
            if 'game' not in columns:
                db.execute('ALTER TABLE profiles ADD COLUMN game TEXT')

    @contextmanager
    def connect(self):
        db = sqlite3.connect(self.path, timeout=10)
        db.row_factory = sqlite3.Row
        try:
            with db:
                yield db
        finally:
            db.close()

    def login(self, uid, password):
        if not uid or uid != uid.strip() or len(uid) > 64:
            raise ValueError("账号需要 1-64 个字符，首尾不能有空格")
        if not 6 <= len(password) <= 128:
            raise ValueError("密码需要 6-128 个字符；首次登录自动创建本地账号")
        session = secrets.token_bytes(16)
        with self.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT * FROM profiles WHERE uid=?', (uid,)).fetchone()
            if row and not row['password_hash']:
                # Never allow claiming a profile created by the old unauthenticated prototype.
                raise ValueError('此账号属于早期无密码测试数据，请使用新的本地账号')
            salt = row['password_salt'] if row else secrets.token_bytes(16)
            digest = hashlib.scrypt(password.encode(), salt=salt, n=16384, r=8, p=1, dklen=32)
            if row:
                if not hmac.compare_digest(digest, row['password_hash']):
                    raise ValueError('账号或密码错误')
                db.execute('UPDATE profiles SET session=? WHERE id=?', (session, row['id']))
            else:
                db.execute('INSERT INTO profiles(uid, session, password_salt, password_hash) VALUES (?,?,?,?)',
                           (uid, session, salt, digest))
            return dict(db.execute("SELECT * FROM profiles WHERE uid=?", (uid,)).fetchone())

    def authorize(self, player_id, session):
        with self.connect() as db:
            row = db.execute("SELECT * FROM profiles WHERE id=?", (player_id,)).fetchone()
        if row is None or not hmac.compare_digest(bytes(row["session"]), session):
            raise PermissionError("Invalid local session")
        return dict(row)

    def find(self, uid):
        with self.connect() as db:
            row = db.execute("SELECT * FROM profiles WHERE uid=?", (uid,)).fetchone()
        return dict(row) if row else None

    def confirm_full_client(self, player_id):
        # Local server has no distribution-channel download promotion. Persist verification only.
        with self.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT game FROM profiles WHERE id=?', (player_id,)).fetchone()
            game = json.loads(row['game'])
            game['full_client_verified'] = True
            db.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), player_id))

    def move(self, player_id, point):
        if any(not math.isfinite(v) or abs(v) > 10000 for v in point.values()):
            raise ValueError('Invalid scene position')
        with self.connect() as db:
            db.execute('BEGIN IMMEDIATE')
            row = db.execute('SELECT game FROM profiles WHERE id=?', (player_id,)).fetchone()
            game = json.loads(row['game'])
            game['position']['current'] = point
            db.execute('UPDATE profiles SET game=? WHERE id=?', (json.dumps(game), player_id))

    def create_player(self, player_id, name, hero_id='hero_043'):
        name = name.strip()
        if not 1 <= len(name) <= 20:
            raise ValueError("Player name must contain 1-20 characters")
        if hero_id not in {'hero_001', *(f'hero_{i:03d}' for i in range(43, 51))}:
            raise ValueError('不支持的初始角色')
        hero_guid = f'{player_id}-hero-1'
        player = {"playerId":player_id, "name":name, "level":1, "exp":0,
                  "coin":0, "diamond":0, "energy":100, "stamina":100,
                  "warriorBagSize":100, "equipBagSize":100, "petBagSize":100,
                  "currentWorldSeedId":1, "noviceGuideSchedule":0, 'heroWarId':hero_guid}
        game = {
            'warriors':[{'id':hero_id, 'guid':hero_guid, 'level':1, 'exp':0,
                         'talent':0, 'training':{}, 'tempTraining':{}, 'breached':{}, 'awake':{}}],
            'formation':[{'formatPos':1, 'battlePos':1,
                          'things':[{'pos':0, 'guid':hero_guid, 'id':hero_id}]}],
            'items':[], 'quests':[], 'kv':[],
            'copy':{'currentChapterId':'1', 'currentMissionId':'1', 'passedCopyInfo':[], 'passedChapterInfo':[]},
            'position':{'playerId':player_id, 'scene':'town_main', 'current':{'x':0, 'z':0}},
        }
        now = int(time.time()*1000)
        with self.connect() as db:
            db.execute("UPDATE profiles SET player=?, game=?, created_at=? WHERE id=? AND player IS NULL",
                       (json.dumps(player, ensure_ascii=False), json.dumps(game), now, player_id))
            row = db.execute("SELECT * FROM profiles WHERE id=?", (player_id,)).fetchone()
        return dict(row)

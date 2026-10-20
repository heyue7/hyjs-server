"""Build the previous release; adapt credentials only for pre-security revisions."""
from pathlib import Path
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[1]
BASE = '35f895fd5f856a225541435607e03bac930c90d6'

def build(output):
    output = Path(output).resolve()
    def run(*args, **kwargs):
        return subprocess.run(['rtk', 'proxy', *map(str, args)], check=True, capture_output=True, **kwargs)
    with tempfile.TemporaryDirectory(prefix='hyjs-auth-rollback-') as temporary:
        work = Path(temporary)
        archive = work / 'source.tar'
        archive.write_bytes(run('git', 'archive', BASE, 'hyjs-server', 'analysis/tables', cwd=ROOT).stdout)
        with tarfile.open(archive) as source:
            source.extractall(work, filter='data')
        source = work / 'hyjs-server'
        if (source / 'utils/passwordCredential.go').is_file():
            # The previous release already supports current credentials. Keep its
            # complete activity/security behavior unchanged for performance rollback.
            run('go', 'test', './service', '-run', '^(TestAdminAuthBootstrapSessionCSRFAndRoles|TestAdminAuthPersistentCooldownAndPasswordChange|TestGlobalAccountsLegacyMigrationAndWalletOrders)$', '-count=1', cwd=source)
            environment = dict(os.environ, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0', GOTOOLCHAIN='auto')
            run('go', 'build', '-trimpath', '-buildvcs=false', '-o', output, '.', cwd=source, env=environment)
            record = {'baseBusinessRevision': BASE, 'SHA256': hashlib.sha256(output.read_bytes()).hexdigest(), 'newAndLegacyCredentialLoginTested': True, 'scope': 'Unmodified previous release including activity batch29 and security batch28'}
            output.with_suffix('.json').write_text(json.dumps(record, indent=2) + '\n')
            return record
        for name in ('utils/passwordCredential.go', 'constUtil/passwordSecurity.go'):
            shutil.copy2(ROOT / 'hyjs-server' / name, source / name)

        path = source / 'service/profileService.go'
        text = path.read_text().replace('"context"', '"context"\n"bytes"\n"hyjs-server/utils"', 1)
        start = text.index('\t\t\tdigest, err := scrypt.Key(')
        end = text.index('\n\t\t\trow.PasswordHash = digest', start)
        text = text[:start] + '''
            var digest []byte
            if found == nil && bytes.HasPrefix(row.PasswordHash, []byte("$argon2id$")) {
                matched,_,err:=utils.VerifyPasswordCredential(ctx,password,row.PasswordSalt,row.PasswordHash)
                if err!=nil{return err};if !matched{return errors.New("账号或密码错误")}
                digest=append([]byte{},row.PasswordHash...)
            } else {
                var err error
                digest,err=scrypt.Key([]byte(password),row.PasswordSalt,16384,8,1,32)
                if err!=nil{return err}
                if found==nil && subtle.ConstantTimeCompare(digest,row.PasswordHash)!=1{return errors.New("账号或密码错误")}
            }''' + text[end:]
        path.write_text(text)

        path = source / 'service/globalGameAccountService.go'
        text = path.read_text().replace('"golang.org/x/crypto/scrypt"', '"hyjs-server/utils"')
        text = text.replace('func globalAccountPasswordMatches(account', 'func globalAccountPasswordMatches(ctx context.Context, account')
        text = text.replace('globalAccountPasswordMatches(&account,', 'globalAccountPasswordMatches(ctx, &account,')
        old = '''digest, err := scrypt.Key([]byte(password), account.PasswordSalt, 16384, 8, 1, 32)
	return err == nil && subtle.ConstantTimeCompare(digest, account.PasswordHash) == 1'''
        assert old in text
        text = text.replace(old, '''matched,_,err:=utils.VerifyPasswordCredential(ctx,password,account.PasswordSalt,account.PasswordHash)
        return err==nil && matched''', 1)
        path.write_text(text)

        path = source / 'service/adminAuthService.go'
        text = path.read_text().replace('"context"', '"context"\n"bytes"\n"encoding/base64"\n"hyjs-server/utils"', 1)
        text = text.replace('bcrypt.CompareHashAndPassword(', 'rollbackComparePassword(ctx, ')
        text += '''
// rollbackComparePassword preserves both pre-security bcrypt and new PHC identities.
func rollbackComparePassword(ctx context.Context, encoded,password []byte) error {
 if !bytes.HasPrefix(encoded,[]byte("$argon2id$")){return bcrypt.CompareHashAndPassword(encoded,password)}
 parts:=strings.Split(string(encoded),"$");if len(parts)!=6{return constUtil.ErrAdminCredentials}
 salt,err:=base64.RawStdEncoding.DecodeString(parts[4]);if err!=nil{return constUtil.ErrAdminCredentials}
 matched,_,err:=utils.VerifyPasswordCredential(ctx,string(password),salt,encoded)
 if err!=nil{return err};if !matched{return constUtil.ErrAdminCredentials};return nil
}
'''
        path.write_text(text)
        (source / 'service/rollbackCompatibility_test.go').write_text('''package service
import("bytes";"context";"path/filepath";"testing";"hyjs-server/db";"hyjs-server/utils";"hyjs-server/constUtil";"golang.org/x/crypto/bcrypt";"golang.org/x/crypto/scrypt")
func TestRollbackCompatibility(t *testing.T){
 ctx:=context.Background();if err:=db.InitGame(ctx,filepath.Join(t.TempDir(),"rollback.db"));err!=nil{t.Fatal(err)}
 c,_:=db.GetIns().DB();defer c.Close()
 salt,encoded,err:=utils.NewPasswordCredential(ctx,"new-game-password");if err!=nil{t.Fatal(err)}
 row:=db.Profile{Uid:"rollback-game",PasswordSalt:salt,PasswordHash:encoded,Session:bytes.Repeat([]byte{1},16)}
 if err:=db.GetIns().Create(&row).Error;err!=nil{t.Fatal(err)}
 if _,err:=LoginAccount(ctx,row.Uid,"new-game-password");err!=nil{t.Fatal("legacy profile",err)}
 s:=&GameService{};if err:=s.InitializeGlobalGameAccounts(ctx);err!=nil{t.Fatal(err)}
 if _,err:=LoginAccount(ctx,row.Uid,"new-game-password");err!=nil{t.Fatal("global account",err)}
 if _,err:=LoginAccount(ctx,row.Uid,"wrong-password");err==nil{t.Fatal("bad game password accepted")}
 if err:=AdminInitialize(ctx,"");err!=nil{t.Fatal(err)}
 _,encoded,err=utils.NewPasswordCredential(ctx,"new-admin-password");if err!=nil{t.Fatal(err)}
 admin:=db.AdminUser{Username:"rollback-owner",PasswordHash:string(encoded),Role:constUtil.AdminRoleAdmin,Enabled:true}
 if err:=db.GetIns().Create(&admin).Error;err!=nil{t.Fatal(err)}
 if _,err:=AdminLogin(ctx,admin.Username,"new-admin-password");err!=nil{t.Fatal("admin",err)}
 if _,err:=AdminLogin(ctx,admin.Username,"wrong-password");err==nil{t.Fatal("bad admin password accepted")}
 oldHash,err:=bcrypt.GenerateFromPassword([]byte("legacy-admin-password"),bcrypt.DefaultCost);if err!=nil{t.Fatal(err)}
 oldAdmin:=db.AdminUser{Username:"legacy-owner",PasswordHash:string(oldHash),Role:constUtil.AdminRoleAdmin,Enabled:true}
 if err:=db.GetIns().Create(&oldAdmin).Error;err!=nil{t.Fatal(err)}
 if _,err:=AdminLogin(ctx,oldAdmin.Username,"legacy-admin-password");err!=nil{t.Fatal("old bcrypt",err)}
 oldSalt:=bytes.Repeat([]byte{7},16);oldDigest,err:=scrypt.Key([]byte("old-game-password"),oldSalt,16384,8,1,32);if err!=nil{t.Fatal(err)}
 if !globalAccountPasswordMatches(ctx,&db.GameAccount{PasswordSalt:oldSalt,PasswordHash:oldDigest},"old-game-password"){t.Fatal("old scrypt")}
}
''')
        run('gofmt', '-w', source / 'service/profileService.go', source / 'service/globalGameAccountService.go', source / 'service/adminAuthService.go', source / 'service/rollbackCompatibility_test.go')
        run('go', 'test', './service', '-run', '^TestRollbackCompatibility$', '-count=1', cwd=source)
        environment = dict(os.environ, GOOS='linux', GOARCH='amd64', CGO_ENABLED='0', GOTOOLCHAIN='auto')
        run('go', 'build', '-trimpath', '-buildvcs=false', '-o', output, '.', cwd=source, env=environment)
        record = {'baseBusinessRevision': BASE, 'SHA256': hashlib.sha256(output.read_bytes()).hexdigest(), 'newAndLegacyCredentialLoginTested': True, 'scope': 'Three previous authentication files plus current bounded credential verifier; no unrelated business/UI changes'}
        output.with_suffix('.json').write_text(json.dumps(record, indent=2) + '\n')
        return record

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    print(json.dumps(build(parser.parse_args().output)))

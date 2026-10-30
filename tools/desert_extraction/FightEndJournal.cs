using System;
using System.Globalization;
using System.Security.Cryptography;
using System.Text;
using ProtoBuf.Message;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Only an original protobuf end request and public identifiers are stored.
    // No login token or client-calculated reward is persisted.
    [Serializable] public sealed class FightEndRecord
    {
        public int version,opcode;
        public bool resumeApproved;
        public long playerId;
        public string origin,fightId,inputHash,payloadBase64,payloadHash;
    }
    public static class FightEndJournal
    {
        const int MaxPayloadBytes=262144;
        static string Key(FieldWire.Session owner)
        {
            if (owner==null || owner.playerId<=0 || String.IsNullOrEmpty(owner.origin)) throw new ArgumentException("Missing fight owner");
            return "hyjs.ggo.fight.end.v4."+Uri.EscapeDataString(owner.origin)+"."+owner.playerId.ToString(CultureInfo.InvariantCulture);
        }
        static bool LowerHex(string value,int count)
        {
            if (value==null || value.Length!=count) return false;
            foreach (char c in value) if (!((c>='0' && c<='9') || (c>='a' && c<='f'))) return false;
            return true;
        }
        static string Digest(byte[] bytes)
        {
            using (SHA256 h=SHA256.Create())
            {
                byte[] value=h.ComputeHash(bytes); var text=new StringBuilder(value.Length*2);
                foreach (byte b in value) text.Append(b.ToString("x2",CultureInfo.InvariantCulture));
                return text.ToString();
            }
        }
        static string MessageFightId(int opcode,object message)
        {
            if (opcode==30049 && message is CSSidePveFightEnd) return ((CSSidePveFightEnd)message).fightId;
            if (opcode==30085 && message is CSSidePveBossFightEnd) return ((CSSidePveBossFightEnd)message).fightId;
            throw new InvalidOperationException("Unexpected original fight end type");
        }
        static byte[] Decode(FightEndRecord r)
        {
            byte[] payload=Convert.FromBase64String(r.payloadBase64);
            if (payload.Length==0 || payload.Length>MaxPayloadBytes || !LowerHex(r.payloadHash,64) || Digest(payload)!=r.payloadHash)
                throw new InvalidOperationException("Invalid persisted fight payload");
            object message=PBConvert.toObject(r.opcode,payload);
            if (message==null || MessageFightId(r.opcode,message)!=r.fightId)
                throw new InvalidOperationException("Persisted fight ID differs from payload");
            return payload;
        }
        static void Validate(FieldWire.Session owner,FightEndRecord r)
        {
            if (r==null || r.version!=1 || r.playerId!=owner.playerId || r.origin!=owner.origin ||
                !LowerHex(r.fightId,24) || (r.opcode!=30049 && r.opcode!=30085) ||
                (!String.IsNullOrEmpty(r.inputHash) && !LowerHex(r.inputHash,64)) ||
                (r.resumeApproved && !LowerHex(r.inputHash,64)) ||
                String.IsNullOrEmpty(r.payloadBase64)) throw new InvalidOperationException("Invalid pending fight record");
            Decode(r);
        }
        public static FightEndRecord Load(FieldWire.Session owner)
        {
            string raw=PlayerPrefs.GetString(Key(owner),""); if (String.IsNullOrEmpty(raw)) return null;
            try { FightEndRecord value=JsonUtility.FromJson<FightEndRecord>(raw); Validate(owner,value); return value; }
            catch (Exception e) { throw new InvalidOperationException("Pending fight record requires recovery",e); }
        }
        static void Save(FieldWire.Session owner,FightEndRecord value)
        {
            Validate(owner,value); string key=Key(owner), raw=JsonUtility.ToJson(value);
            PlayerPrefs.SetString(key,raw); PlayerPrefs.Save();
            if (PlayerPrefs.GetString(key,"")!=raw) throw new InvalidOperationException("Fight record was not saved");
        }
        public static FightEndRecord Capture(FieldWire.Session owner,int opcode,object message,string inputHash)
        { return Capture(owner,opcode,message,inputHash,false); }
        public static FightEndRecord Capture(FieldWire.Session owner,int opcode,object message,string inputHash,bool resumeApproved)
        {
            string fight=MessageFightId(opcode,message);
            if (!LowerHex(fight,24) || (!String.IsNullOrEmpty(inputHash) && !LowerHex(inputHash,64)))
                throw new InvalidOperationException("Invalid fight binding");
            byte[] payload=PBConvert.toBytes(opcode,message);
            if (payload==null || payload.Length==0 || payload.Length>MaxPayloadBytes) throw new InvalidOperationException("Original fight payload unavailable");
            var candidate=new FightEndRecord { version=1,opcode=opcode,playerId=owner.playerId,origin=owner.origin,
                fightId=fight,inputHash=inputHash ?? "",resumeApproved=resumeApproved,payloadBase64=Convert.ToBase64String(payload),payloadHash=Digest(payload) };
            FightEndRecord old=Load(owner);
            if (old!=null)
            {
                if (old.fightId!=candidate.fightId || old.opcode!=opcode || old.payloadHash!=candidate.payloadHash ||
                    old.payloadBase64!=candidate.payloadBase64 || (!String.IsNullOrEmpty(old.inputHash) && !String.IsNullOrEmpty(candidate.inputHash) && old.inputHash!=candidate.inputHash))
                    throw new InvalidOperationException("An earlier fight end remains pending");
                if (!String.IsNullOrEmpty(old.inputHash)) return old;
                if (String.IsNullOrEmpty(candidate.inputHash)) return old;
            }
            Save(owner,candidate); return candidate;
        }
        public static FightEndRecord Bind(FieldWire.Session owner,FightEndRecord value,string inputHash)
        {
            if (!LowerHex(inputHash,64)) throw new InvalidOperationException("Invalid server fight input hash");
            FightEndRecord old=Load(owner);
            if (old==null || value==null || old.fightId!=value.fightId || old.payloadHash!=value.payloadHash ||
                (!String.IsNullOrEmpty(old.inputHash) && old.inputHash!=inputHash)) throw new InvalidOperationException("Fight binding changed");
            if (String.IsNullOrEmpty(old.inputHash)) { old.inputHash=inputHash; Save(owner,old); }
            return old;
        }
        public static object Message(FieldWire.Session owner,FightEndRecord value)
        {
            Validate(owner,value); FightEndRecord current=Load(owner);
            if (current==null || current.fightId!=value.fightId || current.opcode!=value.opcode ||
                current.inputHash!=value.inputHash || current.payloadHash!=value.payloadHash) throw new InvalidOperationException("Fight record changed");
            return PBConvert.toObject(current.opcode,Decode(current));
        }
        public static void Clear(FieldWire.Session owner,FightEndRecord value)
        {
            FightEndRecord current=Load(owner);
            if (current==null || value==null || current.fightId!=value.fightId || current.opcode!=value.opcode ||
                current.inputHash!=value.inputHash || current.payloadHash!=value.payloadHash) throw new InvalidOperationException("Fight record changed before confirmation");
            string key=Key(owner), raw=PlayerPrefs.GetString(key,"");
            try { PlayerPrefs.DeleteKey(key); PlayerPrefs.Save(); }
            catch (Exception) { PlayerPrefs.SetString(key,raw); throw; }
        }
    }
}

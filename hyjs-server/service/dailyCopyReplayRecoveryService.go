package service

import (
	"errors"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

func dailyCopySameEndCore(a, b *pb.CSDailyCopyFightEnd) bool {
	if a == nil || b == nil || a.GetOptRecord() == nil || b.GetOptRecord() == nil {
		return false
	}
	x, y := a.GetOptRecord(), b.GetOptRecord()
	return a.GetFightId() == b.GetFightId() && a.GetChapterId() == b.GetChapterId() && a.GetIsWin() == b.GetIsWin() && x.GetFinalRoundNum() == y.GetFinalRoundNum() && x.GetAttackerTotalLeftHp() == y.GetAttackerTotalLeftHp() && x.GetDefenderTotalLeftHp() == y.GetDefenderTotalLeftHp()
}

// Recover only the persisted owner/result proof if a malformed nested playback
// operation prevents protobuf decoding. The outer frame and core fields must
// remain intact. No saved recording or business receipt is rewritten.
func dailyCopyRecoverEndCore(raw []byte) (*pb.CSDailyCopyFightEnd, error) {
	end := &pb.CSDailyCopyFightEnd{}
	if err := proto.Unmarshal(raw, end); err == nil {
		return end, nil
	}
	bad := func() (*pb.CSDailyCopyFightEnd, error) { return nil, errors.New(constUtil.DailyCopyReplayError) }
	end = &pb.CSDailyCopyFightEnd{}
	seen := map[protowire.Number]bool{}
	for len(raw) > 0 {
		number, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return bad()
		}
		raw = raw[n:]
		valueLen := protowire.ConsumeFieldValue(number, typ, raw)
		if valueLen < 0 {
			return bad()
		}
		if number >= 1 && number <= 4 {
			if seen[number] {
				return bad()
			}
			seen[number] = true
			switch number {
			case 1, 3, 4:
				if typ != protowire.BytesType {
					return bad()
				}
				data, size := protowire.ConsumeBytes(raw)
				if size < 0 {
					return bad()
				}
				switch number {
				case 1:
					end.ChapterId = proto.String(string(data))
				case 3:
					op, err := dailyCopyRecoverOperateCore(data)
					if err != nil {
						return bad()
					}
					end.OptRecord = op
				case 4:
					end.FightId = proto.String(string(data))
				}
			case 2:
				if typ != protowire.VarintType {
					return bad()
				}
				value, size := protowire.ConsumeVarint(raw)
				if size < 0 || value > 1 {
					return bad()
				}
				end.IsWin = proto.Int32(int32(value))
			}
		}
		raw = raw[valueLen:]
	}
	if !seen[1] || !seen[3] || !seen[4] {
		return bad()
	}
	return end, nil
}

func dailyCopyRecoverOperateCore(raw []byte) (*pb.BattleOperate, error) {
	op := &pb.BattleOperate{}
	seen := map[protowire.Number]bool{}
	for len(raw) > 0 {
		number, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return nil, errors.New(constUtil.DailyCopyReplayError)
		}
		raw = raw[n:]
		if number >= 4 {
			// Generated storage emits the round and HP fields before playback.
			break
		}
		if number < 1 || seen[number] {
			return nil, errors.New(constUtil.DailyCopyReplayError)
		}
		seen[number] = true
		switch number {
		case 1:
			if typ != protowire.VarintType {
				return nil, errors.New(constUtil.DailyCopyReplayError)
			}
			value, size := protowire.ConsumeVarint(raw)
			if size < 0 || value > 40 {
				return nil, errors.New(constUtil.DailyCopyReplayError)
			}
			op.FinalRoundNum = proto.Int32(int32(value))
			raw = raw[size:]
		case 2, 3:
			if typ != protowire.Fixed64Type {
				return nil, errors.New(constUtil.DailyCopyReplayError)
			}
			value, size := protowire.ConsumeFixed64(raw)
			if size < 0 {
				return nil, errors.New(constUtil.DailyCopyReplayError)
			}
			hp := math.Float64frombits(value)
			if number == 2 {
				op.AttackerTotalLeftHp = proto.Float64(hp)
			} else {
				op.DefenderTotalLeftHp = proto.Float64(hp)
			}
			raw = raw[size:]
		}
	}
	if !seen[1] {
		return nil, errors.New(constUtil.DailyCopyReplayError)
	}
	return op, nil
}

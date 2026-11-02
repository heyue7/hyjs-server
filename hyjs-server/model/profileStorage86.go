package model

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// Only the cold roster is compressed. Position and all existing JSON query
// paths remain visible, while movement no longer rewrites the expanded roster.
// Legacy arrays remain readable and the envelope authenticates its original JSON.
type storedWarriors86 struct {
	Codec  string `json:"codec"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"data"`
}

func encodeWarriors86(warriors []*pb.Warrior) (json.RawMessage, error) {
	raw, err := json.Marshal(warriors)
	if err != nil {
		return raw, err
	}
	return compressWarriorsJSON86(raw)
}

func compressWarriorsJSON86(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) < constUtil.ProfileWarriorsCompressMin86 {
		return raw, nil
	}
	if len(raw) > constUtil.ProfileWarriorsMaxBytes86 {
		return nil, errors.New("角色库存超过存档大小上限")
	}
	var compressed bytes.Buffer
	writer, err := zlib.NewWriterLevel(&compressed, zlib.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(raw); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	envelope, err := json.Marshal(storedWarriors86{Codec: constUtil.ProfileWarriorsCodec86, Bytes: int64(len(raw)), SHA256: hex.EncodeToString(digest[:]), Data: compressed.Bytes()})
	if err != nil {
		return nil, err
	}
	if len(envelope) >= len(raw) {
		return raw, nil
	}
	return envelope, nil
}

func expandWarriorsJSON86(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, errors.New("角色库存存档为空")
	}
	if raw[0] == '{' {
		// Reject an oversized base64 envelope before allocating its decoded data.
		if len(raw) > ((constUtil.ProfileWarriorsMaxBytes86+2)/3)*4+1024 {
			return nil, errors.New("角色库存压缩存档超过大小上限")
		}
		var envelope storedWarriors86
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil {
			return nil, err
		}
		if envelope.Codec != constUtil.ProfileWarriorsCodec86 || envelope.Bytes < 1 || envelope.Bytes > constUtil.ProfileWarriorsMaxBytes86 || len(envelope.Data) == 0 || len(envelope.Data) > constUtil.ProfileWarriorsMaxBytes86 || len(envelope.SHA256) != 64 {
			return nil, errors.New("角色库存压缩存档无效")
		}
		source := bytes.NewReader(envelope.Data)
		reader, err := zlib.NewReader(source)
		if err != nil {
			return nil, err
		}
		plain, readErr := io.ReadAll(io.LimitReader(reader, envelope.Bytes+1))
		closeErr := reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		digest := sha256.Sum256(plain)
		if int64(len(plain)) != envelope.Bytes || source.Len() != 0 || hex.EncodeToString(digest[:]) != envelope.SHA256 {
			return nil, errors.New("角色库存存档校验失败")
		}
		raw = plain
	}
	if !json.Valid(raw) || raw[0] != '[' && !bytes.Equal(raw, []byte("null")) {
		return nil, errors.New("角色库存存档格式无效")
	}
	return raw, nil
}

func decodeWarriors86(raw json.RawMessage) ([]*pb.Warrior, error) {
	plain, err := expandWarriorsJSON86(raw)
	if err != nil {
		return nil, err
	}
	var warriors []*pb.Warrior
	if err := json.Unmarshal(plain, &warriors); err != nil {
		return nil, err
	}
	return warriors, nil
}

// ExpandProfileGame86 retains every original JSON field. Maintenance tools and
// the admin GUID-aware merge use this before inspecting a stored roster.
func ExpandProfileGame86(raw []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	roster, ok := fields["warriors"]
	if !ok {
		return raw, nil
	}
	plain, err := expandWarriorsJSON86(roster)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(roster), plain) {
		return raw, nil
	}
	fields["warriors"] = plain
	return json.Marshal(fields)
}

// CompactProfileGame86 compresses raw roster JSON without removing unknown
// fields, allowing operational edits and a lossless legacy-export rollback.
func CompactProfileGame86(raw []byte) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	roster, ok := fields["warriors"]
	if !ok {
		return raw, nil
	}
	plain, err := expandWarriorsJSON86(roster)
	if err != nil {
		return nil, err
	}
	compact, err := compressWarriorsJSON86(plain)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(bytes.TrimSpace(roster), compact) {
		return raw, nil
	}
	fields["warriors"] = compact
	return json.Marshal(fields)
}

// MarshalProfileGame86 is used only at the persistence boundary. Ordinary
// JSON exports/clones and every original protocol object keep their old shape.
// Identical state produces identical stored bytes on every save.
func MarshalProfileGame86(game *GameState) ([]byte, error) {
	roster, err := encodeWarriors86(game.Warriors)
	if err != nil {
		return nil, err
	}
	return marshalProfileWithRoster86(game, roster)
}

// MarshalProfileGame86Reusing keeps a verified compressed roster when the
// transaction changed other fields only. A damaged previous roster is rejected
// and changed rosters still pass through the ordinary bounded compressor.
func MarshalProfileGame86Reusing(game *GameState, previous []byte) ([]byte, error) {
	if !bytes.Contains(previous, []byte(constUtil.ProfileWarriorsCodec86)) {
		return MarshalProfileGame86(game)
	}
	var stored struct {
		Warriors json.RawMessage `json:"warriors"`
	}
	if err := json.Unmarshal(previous, &stored); err != nil {
		return nil, err
	}
	roster := bytes.TrimSpace(stored.Warriors)
	if len(roster) == 0 || roster[0] != '{' {
		return MarshalProfileGame86(game)
	}
	plain, err := expandWarriorsJSON86(roster)
	if err != nil {
		return nil, err
	}
	current, err := json.Marshal(game.Warriors)
	if err != nil {
		return nil, err
	}
	if bytes.Equal(plain, current) {
		return marshalProfileWithRoster86(game, roster)
	}
	roster, err = compressWarriorsJSON86(current)
	if err != nil {
		return nil, err
	}
	return marshalProfileWithRoster86(game, roster)
}

func marshalProfileWithRoster86(game *GameState, roster json.RawMessage) ([]byte, error) {
	type plainGameState GameState
	if len(roster) == 0 || roster[0] != '{' {
		// Keep historical field order and bytes for rosters that need no codec.
		return json.Marshal(game)
	}
	return json.Marshal(struct {
		*plainGameState
		Warriors json.RawMessage `json:"warriors"`
	}{plainGameState: (*plainGameState)(game), Warriors: roster})
}

// UnmarshalJSON supports both historical arrays and bounded compressed rosters.
// Legacy arrays remain the ordinary export format; compressed data is expanded
// before any gameplay reader can use the roster.
func (game *GameState) UnmarshalJSON(raw []byte) error {
	type plainGameState GameState
	if !bytes.Contains(raw, []byte(constUtil.ProfileWarriorsCodec86)) {
		return json.Unmarshal(raw, (*plainGameState)(game))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	roster, ok := fields["warriors"]
	if !ok {
		return json.Unmarshal(raw, (*plainGameState)(game))
	}
	plain, err := expandWarriorsJSON86(roster)
	if err != nil {
		return err
	}
	if bytes.Equal(bytes.TrimSpace(roster), plain) {
		return json.Unmarshal(raw, (*plainGameState)(game))
	}
	delete(fields, "warriors")
	otherFields, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(otherFields, (*plainGameState)(game)); err != nil {
		return err
	}
	return json.Unmarshal(plain, &game.Warriors)
}

package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FrozenJSONWindow 为同一冻结对象的开始、结束、最终领奖毫秒三元组。
type FrozenJSONWindow struct{ StartAt, EndAt, ClaimEndAt int64 }

// FrozenJSONIdentity 指定已存在的字符串身份；路径逐段给定，不解释点号或自动生成ID。
type FrozenJSONIdentity struct {
	Path     []string
	Expected string
}

// FrozenJSONWindowPatch 将调用者明确确认的字段路径、身份及CAS窗口绑定到一次纯转换。
// FormJSON使用startAt/endAt/claimEndAt；Period冻结使用startAt/endAt/rewardEndAt。
// GameplayPool无Frozen列；实际招募来源快照的pool对象使用pool.startAt/endAt/rewardEndAt及pool.id。
type FrozenJSONWindowPatch struct {
	StartPath, EndPath, ClaimEndPath []string
	Identities                       []FrozenJSONIdentity
	Expected, Target                 FrozenJSONWindow
	MinMillis, MaxMillis             int64
	SkipEmpty                        bool
}

// PatchFrozenJSONWindow 只改指定时间字段，保留其余JSON语义；未变更时返回原文副本。
// 调用者仍须用原文或版本进行数据库UPDATE CAS；本函数不建立跨事务锁或写数据库。
func PatchFrozenJSONWindow(raw []byte, patch FrozenJSONWindowPatch) ([]byte, bool, error) {
	fail := func(message string) ([]byte, bool, error) { return nil, false, errors.New(message) }
	if patch.MinMillis <= 0 || patch.MaxMillis < patch.MinMillis || patch.MaxMillis > 253402300799999 {
		return fail("冻结时间允许范围无效")
	}
	validWindow := func(w FrozenJSONWindow) bool {
		return w.StartAt >= patch.MinMillis && w.StartAt < w.EndAt && w.EndAt <= w.ClaimEndAt && w.ClaimEndAt <= patch.MaxMillis
	}
	if !validWindow(patch.Expected) || !validWindow(patch.Target) {
		return fail("冻结时间三元组无效或出界")
	}
	paths := [][]string{patch.StartPath, patch.EndPath, patch.ClaimEndPath}
	seen := map[string]bool{}
	validatePath := func(path []string) bool {
		if len(path) == 0 {
			return false
		}
		for _, part := range path {
			if part == "" {
				return false
			}
		}
		encoded, _ := json.Marshal(path)
		key := string(encoded)
		if seen[key] {
			return false
		}
		seen[key] = true
		return true
	}
	for _, path := range paths {
		if !validatePath(path) {
			return fail("冻结时间字段映射无效")
		}
	}
	if len(patch.Identities) == 0 {
		return fail("冻结身份约束缺失")
	}
	for _, identity := range patch.Identities {
		if identity.Expected == "" || !validatePath(identity.Path) {
			return fail("冻结身份映射无效")
		}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		if patch.SkipEmpty {
			return append([]byte(nil), raw...), false, nil
		}
		return fail("冻结JSON为空")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	root, err := readFrozenJSONValue(decoder, 0)
	if err != nil {
		return nil, false, fmt.Errorf("冻结JSON无效: %w", err)
	}
	if _, err = decoder.Token(); !errors.Is(err, io.EOF) {
		return fail("冻结JSON存在尾随内容")
	}
	object, ok := root.(map[string]any)
	if !ok {
		return fail("冻结JSON必须是对象")
	}
	locate := func(path []string) (map[string]any, string, error) {
		parent := object
		for _, part := range path[:len(path)-1] {
			next, ok := parent[part].(map[string]any)
			if !ok {
				return nil, "", errors.New("冻结字段父路径不存在或类型无效")
			}
			parent = next
		}
		field := path[len(path)-1]
		if _, ok := parent[field]; !ok {
			return nil, "", errors.New("冻结字段缺失")
		}
		return parent, field, nil
	}
	for _, identity := range patch.Identities {
		parent, key, err := locate(identity.Path)
		if err != nil {
			return nil, false, err
		}
		value, ok := parent[key].(string)
		if !ok || value != identity.Expected {
			return fail("冻结身份不匹配")
		}
	}
	var current [3]int64
	for i, path := range paths {
		parent, key, err := locate(path)
		if err != nil {
			return nil, false, err
		}
		number, ok := parent[key].(json.Number)
		if !ok {
			return fail("冻结时间字段必须是整数")
		}
		value, err := strconv.ParseInt(string(number), 10, 64)
		if err != nil {
			return fail("冻结时间字段必须是int64整数")
		}
		current[i] = value
	}
	expected := [3]int64{patch.Expected.StartAt, patch.Expected.EndAt, patch.Expected.ClaimEndAt}
	target := [3]int64{patch.Target.StartAt, patch.Target.EndAt, patch.Target.ClaimEndAt}
	if current == target {
		return append([]byte(nil), raw...), false, nil
	}
	if current != expected {
		return fail("冻结时间CAS冲突")
	}
	for i, path := range paths {
		parent, key, err := locate(path)
		if err != nil {
			return nil, false, err
		}
		parent[key] = json.Number(strconv.FormatInt(target[i], 10))
	}
	result, err := json.Marshal(object)
	if err != nil {
		return nil, false, err
	}
	return result, true, nil
}

// readFrozenJSONValue 拒绝任意层重复键，避免身份或时间字段被解析器覆盖。
func readFrozenJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, errors.New("冻结JSON嵌套过深")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := map[string]any{}
		for decoder.More() {
			token, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := token.(string)
			if !ok {
				return nil, errors.New("冻结JSON键无效")
			}
			if _, exists := object[key]; exists {
				return nil, errors.New("冻结JSON存在重复键")
			}
			value, err := readFrozenJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		close, err := decoder.Token()
		if err != nil || close != json.Delim('}') {
			return nil, errors.New("冻结JSON对象未结束")
		}
		return object, nil
	case '[':
		values := []any{}
		for decoder.More() {
			value, err := readFrozenJSONValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		close, err := decoder.Token()
		if err != nil || close != json.Delim(']') {
			return nil, errors.New("冻结JSON数组未结束")
		}
		return values, nil
	default:
		return nil, fmt.Errorf("冻结JSON分隔符无效: %s", strings.TrimSpace(string(delimiter)))
	}
}

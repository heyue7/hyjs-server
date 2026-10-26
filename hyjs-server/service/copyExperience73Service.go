package service

import (
	_ "embed"
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
)

//go:embed data/copy_experience73.csv
var copyExperience73CSV string

// installCopyExperience73 assigns the approved absolute row values once during
// table loading. Both the original battle and sweep consumers read CopyExp.
// A repeated initialization cannot multiply an already adjusted row.
func (s *GameService) installCopyExperience73() error {
	var rows, err = csv.NewReader(strings.NewReader(copyExperience73CSV)).ReadAll()
	if err != nil || len(rows) != 101 {
		return errors.New("副本经验梯度配置无效")
	}
	var values = make(map[string]string, 100)
	for _, row := range rows[1:] {
		if len(row) != 4 {
			return errors.New("副本经验梯度行无效")
		}
		value, err := strconv.ParseInt(row[2], 10, 32)
		if err != nil || value <= 0 || values[row[0]] != "" {
			return errors.New("副本经验梯度值无效")
		}
		values[row[0]] = row[2]
	}
	for _, row := range s.Tables["CopyExp"] {
		if value, ok := values[row["level"]]; ok {
			row["exp"] = value
		}
	}
	return nil
}

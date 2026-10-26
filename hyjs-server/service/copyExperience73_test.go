package service

import (
	"encoding/csv"
	"math"
	"os"
	"strconv"
	"testing"
)

func TestCopyExperienceGradient73AbsoluteAndCoinUnchanged(t *testing.T) {
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("../../analysis/tables/TableCopyExp.tsv")
	if err != nil {
		t.Fatal(err)
	}
	reader := csv.NewReader(f)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	raw, err := reader.ReadAll()
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	original := map[string]int{}
	for _, row := range raw[5:] {
		if len(row) >= 3 {
			original[row[0]], _ = strconv.Atoi(row[2])
		}
	}
	coins := map[string]string{}
	values := map[string]string{}
	for _, row := range s.Tables["CopyExp"] {
		coins[row["level"]] = row["coin"]
		values[row["level"]] = row["exp"]
	}
	if err := s.installCopyExperience73(); err != nil {
		t.Fatal(err)
	}
	for _, row := range s.Tables["CopyExp"] {
		level, _ := strconv.Atoi(row["level"])
		base := original[row["level"]]
		expected := int(math.Floor(float64(base)*(1.10+2.80*math.Pow(float64(level-1)/99, 3)) + .5))
		if row["coin"] != coins[row["level"]] || row["exp"] != values[row["level"]] || row["exp"] != strconv.Itoa(expected) {
			t.Fatal(row, expected)
		}
	}
}

package onlineconfbot

import (
	"bytes"
	"database/sql"
	"encoding/json"
)

type NullString struct {
	sql.NullString
}

func (ns NullString) MarshalJSON() ([]byte, error) {
	if ns.Valid {
		return json.Marshal(ns.String)
	} else {
		return json.Marshal(nil)
	}
}

func (ns *NullString) UnmarshalJSON(data []byte) error {
	if bytes.Compare(data, []byte("null")) == 0 {
		return nil
	}
	ns.Valid = true
	return json.Unmarshal(data, &ns.String)
}

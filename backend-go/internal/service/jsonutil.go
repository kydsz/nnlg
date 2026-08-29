package service

import (
	"encoding/json"
)

func marshalJSON(v interface{}) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	return json.RawMessage(b), err
}

// toRawJSON 字符串数组转 RawMessage（失败返回空）
func toRawJSON(v []string) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

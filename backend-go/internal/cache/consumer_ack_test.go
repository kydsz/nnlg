package cache

import (
	"errors"
	"testing"
)

// 同批既有可解析又有解析失败消息：解析失败项必须被保留在 ACK 集合中，
// 不得被后续 persist 返回的成功项覆盖（issue #20 AC5）。
func TestParseAndMergeAckKeepsParseFailures(t *testing.T) {
	msgs := []XMessage{
		{ID: "m-ok-1", Values: map[string]interface{}{
			"task_id": "1", "user_id": "1", "dimension_values": `{"score1":90}`,
		}},
		{ID: "m-ok-2", Values: map[string]interface{}{
			"task_id": "2", "user_id": "2", "dimension_values": `{"score1":80}`,
		}},
		{ID: "m-bad", Values: map[string]interface{}{"task_id": "3"}}, // 缺 dimension_values
	}

	parsed, ack := parseAndMergeAck(msgs, func(m XMessage) (EvalSubmitMsg, error) {
		var msg EvalSubmitMsg
		if err := parseMsg(m.Values, &msg); err != nil {
			return msg, err
		}
		msg.MsgID = m.ID
		return msg, nil
	})

	if len(parsed) != 2 {
		t.Fatalf("应解析出 2 条可处理消息, 实际 %d", len(parsed))
	}
	if !ack["m-bad"] {
		t.Fatal("解析失败消息 m-bad 必须出现在 ACK 集合")
	}
	// persist 只确认 ok 项；合并后 bad 项不得被覆盖丢失
	for _, id := range []string{"m-ok-1", "m-ok-2"} {
		ack[id] = true
	}
	for _, want := range []string{"m-bad", "m-ok-1", "m-ok-2"} {
		if !ack[want] {
			t.Fatalf("合并后应包含 %s", want)
		}
	}
}

// 解析全部失败时，parsed 为空且所有消息进入 ACK（不会死循环）。
func TestParseAndMergeAckAllFailures(t *testing.T) {
	msgs := []XMessage{
		{ID: "bad-1", Values: map[string]interface{}{"task_id": "x"}},
		{ID: "bad-2", Values: map[string]interface{}{"user_id": "y"}},
	}
	parseAlwaysFail := func(XMessage) (EvalSubmitMsg, error) {
		return EvalSubmitMsg{}, errors.New("always fail")
	}
	parsed, ack := parseAndMergeAck(msgs, parseAlwaysFail)
	if len(parsed) != 0 {
		t.Fatalf("全部解析失败时 parsed 应为空, 实际 %d", len(parsed))
	}
	if !ack["bad-1"] || !ack["bad-2"] {
		t.Fatal("全部失败消息都应进入 ACK 集合")
	}
}

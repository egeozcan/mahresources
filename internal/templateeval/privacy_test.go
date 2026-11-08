package templateeval

import (
	"encoding/json"
	"testing"
)

func TestRequestSendsNoPrivacyOffByDefault(t *testing.T) {
	c := Case{Kind: KindSlot, Slot: "CustomCSS", Prompt: "p"}
	if _, ok := c.Request(1, 2)["privacyOff"]; ok {
		t.Fatal("the default request leaves Privacy mode on, as the editor does")
	}
	if got := c.RequestWith(1, 2, RequestOptions{PrivacyOff: true})["privacyOff"]; got != true {
		t.Fatalf("privacyOff = %v", got)
	}
}

func TestReportRecordsPrivacy(t *testing.T) {
	raw, _ := json.Marshal(RunReport{Privacy: PrivacyOn})
	var back RunReport
	if err := json.Unmarshal(raw, &back); err != nil || back.Privacy != PrivacyOn {
		t.Fatalf("%v %+v", err, back)
	}
	var old RunReport
	if err := json.Unmarshal([]byte(`{"repeats":1}`), &old); err != nil || old.Privacy != "" {
		t.Fatal("a report from before Privacy mode has no value")
	}
}

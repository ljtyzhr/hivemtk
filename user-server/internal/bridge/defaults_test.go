package bridge

import (
	"testing"
	"time"
)

func TestBridgeDefaultsDocConsistency(t *testing.T) {
	t.Run("HTTPIngestConstants", func(t *testing.T) {
		if HTTPIngestMaxBodySize != 4<<20 {
			t.Errorf("HTTPIngestMaxBodySize 应为 4MB（4<<20），实际 %d", HTTPIngestMaxBodySize)
		}
		if HTTPIngestMaxMessages != 200 {
			t.Errorf("HTTPIngestMaxMessages 应为 200，实际 %d", HTTPIngestMaxMessages)
		}
	})

	t.Run("OnlineCheckConstants", func(t *testing.T) {
		if OnlineGraceWindow != 30*time.Second {
			t.Errorf("OnlineGraceWindow 应为 30s，实际 %v", OnlineGraceWindow)
		}
	})

	t.Run("NonSoftStartup", func(t *testing.T) {
		if HTTPIngestMaxBodySize <= 0 {
			t.Error("HTTPIngestMaxBodySize 必须为正数")
		}
		if HTTPIngestMaxMessages <= 0 {
			t.Error("HTTPIngestMaxMessages 必须为正数")
		}
		if OnlineGraceWindow <= 0 {
			t.Error("OnlineGraceWindow 必须为正数")
		}
	})
}

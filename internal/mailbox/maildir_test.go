package mailbox

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew_DirectoryStructure(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Fatalf("expected error when passing blank string to New(), got err == nil")
	}

	testDir, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("error creating temporary testing dir: %v", err)
	}

	_, err = os.Stat(testDir.tmpDir)
	if err != nil {
		t.Fatalf("`tmpDir` does not exist after New(): %v", err)
	}

	_, err = os.Stat(testDir.newDir)
	if err != nil {
		t.Fatalf("`newDir` does not exist after New(): %v", err)
	}

	_, err = os.Stat(testDir.curDir)
	if err != nil {
		t.Fatalf("`curDir` does not exist after New(): %v", err)
	}

	_, err = os.Stat(testDir.quarantineDir)
	if err != nil {
		t.Fatalf("`quarantineDir` does not exist after New(): %v", err)
	}
}

func TestMaildir_EnqueueAndDrainFIFO(t *testing.T) {
	testDir, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("error creating temporary testing dir: %v", err)
	}

	testMsgOne := &Message{
		Timestamp: time.Now(),
		Sender:    "caduceus-agent1",
		Content:   "This is test message 1",
	}

	testMsgTwo := &Message{
		Timestamp: time.Now(),
		Sender:    "caduceus-agent2",
		Content:   "This is test message 2",
	}

	testMsgThree := &Message{
		Timestamp: time.Now(),
		Sender:    "caduceus-agent3",
		Content:   "This is test message 3",
	}

	err = testDir.Enqueue(*testMsgOne)
	if err != nil {
		t.Fatalf("error enqueueing testMessageOne: %v", err)
	}
	err = testDir.Enqueue(*testMsgTwo)
	if err != nil {
		t.Fatalf("error enqueueing testMessageTwo: %v", err)
	}
	err = testDir.Enqueue(*testMsgThree)
	if err != nil {
		t.Fatalf("error enqueueing testMessageThree: %v", err)
	}

	drained, err := testDir.Drain()
	if err != nil {
		t.Fatalf("error draining `/new`: %v", err)
	}

	if len(drained) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(drained))
	}

	if drained[0].Sender != testMsgOne.Sender || drained[0].Content != testMsgOne.Content {
		t.Fatalf("ordering check: expected testMsgOne to have `caduceus-agent1` sender and content `This is test message 1`, but got Sender: %s, Content: %s", drained[0].Sender, drained[0].Content)
	}
	if drained[1].Sender != testMsgTwo.Sender || drained[1].Content != testMsgTwo.Content {
		t.Fatalf("ordering check: expected testMsgTwo to have `caduceus-agent2` sender and content `This is test message 2`, but got Sender: %s, Content: %s", drained[1].Sender, drained[1].Content)
	}
	if drained[2].Sender != testMsgThree.Sender || drained[2].Content != testMsgThree.Content {
		t.Fatalf("ordering check: expected testMsgThree to have `caduceus-agent3` sender and content `This is test message 3`, but got Sender: %s, Content: %s", drained[2].Sender, drained[2].Content)
	}
}

func TestMaildir_DrainEmptiesQueue(t *testing.T) {
	testDir, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("error creating temporary testing dir: %v", err)
	}

	testMessage := &Message{
		Timestamp: time.Now(),
		Sender:    "caduceus-agent",
		Content:   "This is a test message",
	}

	err = testDir.Enqueue(*testMessage)
	if err != nil {
		t.Fatalf("error enqueueing test message: %v", err)
	}

	drained, err := testDir.Drain()
	if err != nil {
		t.Fatalf("error during initial draining of inbox")
	}

	if len(drained) != 1 {
		t.Fatalf("expected to receive only one message, but got %d", len(drained))
	}

	drainedTwo, err := testDir.Drain()
	if err != nil {
		t.Fatalf("error while attempting second drain of inbox: %v", err)
	}

	if len(drainedTwo) != 0 {
		t.Fatalf("expected `/new` to be empty after second drain, but got %d messages", len(drainedTwo))
	}

	entries, err := os.ReadDir(testDir.newDir)
	if err != nil {
		t.Fatalf("error while reading `/new` after drain: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf("expected 0 entries in `new` after drain, but got %d", len(entries))
	}

}

func TestMaildir_PersistenceAcrossRestarts(t *testing.T) {
	dir := t.TempDir()

	md1, err := New(dir)
	if err != nil {
		t.Fatalf("error creating md1: %v", err)
	}

	messageOne := &Message{
		Timestamp: time.Now(),
		Sender:    "caduceus-agent1",
		Content:   "This is test message 1",
	}
	err = md1.Enqueue(*messageOne)
	if err != nil {
		t.Fatalf("error enqueueing message one: %v", err)
	}

	messageTwo := &Message{
		Timestamp: time.Now(),
		Sender:    "caduceus-agent2",
		Content:   "This is test message 2",
	}

	err = md1.Enqueue(*messageTwo)
	if err != nil {
		t.Fatalf("error enqueueing message two: %v", err)
	}

	md2, err := New(dir)
	if err != nil {
		t.Fatalf("error creating md2: %v", err)
	}

	drained, err := md2.Drain()
	if err != nil {
		t.Fatalf("error draining md2: %v", err)
	}

	if len(drained) != 2 {
		t.Fatalf("expected exactly 2 entries to be drained, got %d", len(drained))
	}

	if drained[0].Sender != messageOne.Sender || drained[0].Content != messageOne.Content {
		t.Fatalf("ordering check: expected message one to have `caduceus-agent1` sender and content `This is test message 1`, but got Sender: %s, Content: %s", drained[0].Sender, drained[0].Content)
	}
	if drained[1].Sender != messageTwo.Sender || drained[1].Content != messageTwo.Content {
		t.Fatalf("ordering check: expected message two to have `caduceus-agent2` sender and content `This is test message 2`, but got Sender: %s, Content: %s", drained[1].Sender, drained[1].Content)
	}

}

func TestMaildir_CorruptMessageQuarantine(t *testing.T) {
	testDir, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("error creating temporary testing dir: %v", err)
	}

	corruptFilename := "corrupt_payload.json"
	corruptPayload := []byte(`{"invalid_json": [unclosed`)
	corruptPath := filepath.Join(testDir.newDir, corruptFilename)

	if err := os.WriteFile(corruptPath, corruptPayload, 0600); err != nil {
		t.Fatalf("failed to write corrupt test file: %v", err)
	}

	validMsg := Message{
		Timestamp: time.Now(),
		Sender:    "valid-agent",
		Content:   "valid test payload",
	}
	if err := testDir.Enqueue(validMsg); err != nil {
		t.Fatalf("failed to enqueue valid message: %v", err)
	}

	drained, err := testDir.Drain()
	if err != nil {
		t.Fatalf("Drain() failed with corrupt message present: %v", err)
	}

	if len(drained) != 1 {
		t.Fatalf("expected 1 valid message drained, got %d", len(drained))
	}
	if drained[0].Sender != validMsg.Sender || drained[0].Content != validMsg.Content {
		t.Fatalf("drained message mismatch: got %+v, want %+v", drained[0], validMsg)
	}

	newEntries, err := os.ReadDir(testDir.newDir)
	if err != nil {
		t.Fatalf("failed to read newDir: %v", err)
	}
	if len(newEntries) != 0 {
		t.Fatalf("expected 0 files remaining in newDir, found %d", len(newEntries))
	}

	quarantineEntries, err := os.ReadDir(testDir.quarantineDir)
	if err != nil {
		t.Fatalf("failed to read quarantineDir: %v", err)
	}
	if len(quarantineEntries) != 1 {
		t.Fatalf("expected 1 file in quarantineDir, found %d", len(quarantineEntries))
	}
	if quarantineEntries[0].Name() != corruptFilename {
		t.Fatalf("expected quarantined file name %s, got %s", corruptFilename, quarantineEntries[0].Name())
	}

	quarantinedData, err := os.ReadFile(filepath.Join(testDir.quarantineDir, corruptFilename))
	if err != nil {
		t.Fatalf("failed to read quarantined file: %v", err)
	}
	if !bytes.Equal(quarantinedData, corruptPayload) {
		t.Fatalf("quarantined content mismatch: got %q, want %q", quarantinedData, corruptPayload)
	}

	secondDrain, err := testDir.Drain()
	if err != nil {
		t.Fatalf("second Drain() failed: %v", err)
	}
	if len(secondDrain) != 0 {
		t.Fatalf("expected 0 messages in second Drain(), got %d", len(secondDrain))
	}
}

func BenchmarkMaildir_Drain(b *testing.B) {
	dir := b.TempDir()
	md, err := New(dir)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	msg := Message{
		ID:        "bench-id",
		Kind:      "test",
		Timestamp: time.Now(),
		Sender:    "bench-sender",
		Content:   "benchmark payload content",
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for j := 0; j < 10; j++ {
			_ = md.Enqueue(msg)
		}
		b.StartTimer()

		drained, err := md.Drain()
		if err != nil {
			b.Fatalf("Drain() failed: %v", err)
		}
		if len(drained) != 10 {
			b.Fatalf("expected 10 drained, got %d", len(drained))
		}
	}
}

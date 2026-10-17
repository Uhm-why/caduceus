package mailbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

var _ Mailbox = (*Maildir)(nil)

type Maildir struct {
	rootDir string
	tmpDir  string
	newDir  string
	curDir  string
	mu      sync.Mutex
	counter atomic.Uint64
}

func New(path string) (*Maildir, error) {
	if path == "" {
		return nil, errors.New("mailbox path cannot be empty")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	tmpDir := filepath.Join(absPath, "tmp")
	newDir := filepath.Join(absPath, "new")
	curDir := filepath.Join(absPath, "cur")

	err = os.MkdirAll(tmpDir, 0700)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(newDir, 0700)
	if err != nil {
		return nil, err
	}

	err = os.MkdirAll(curDir, 0700)
	if err != nil {
		return nil, err
	}

	return &Maildir{
		rootDir: absPath,
		tmpDir:  tmpDir,
		newDir:  newDir,
		curDir:  curDir,
		mu:      sync.Mutex{},
		counter: atomic.Uint64{},
	}, nil
}

func (md *Maildir) generateFilename() string {
	seq := md.counter.Add(1)
	return fmt.Sprintf("%d_%d_%d.json", time.Now().UTC().UnixNano(), os.Getpid(), seq)
}

func (md *Maildir) Enqueue(msg Message) error {
	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	name := md.generateFilename()

	tmpPath := filepath.Join(md.tmpDir, name)
	newPath := filepath.Join(md.newDir, name)

	tmpFile, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}

	_, err = tmpFile.Write(msgBytes)
	if err != nil {
		_ = tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	err = tmpFile.Sync()
	if err != nil {
		_ = tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	err = tmpFile.Close()
	if err != nil {
		return err
	}

	err = os.Rename(tmpPath, newPath)
	if err != nil {
		return err
	}

	return nil

}

func (md *Maildir) Drain() ([]Message, error) {
	md.mu.Lock()
	defer md.mu.Unlock()

	entries, err := os.ReadDir(md.newDir)
	if err != nil {
		return nil, err
	}
	entryList := []string{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		entryList = append(entryList, entry.Name())
	}
	slices.Sort(entryList)

	messageList := []Message{}

	for _, message := range entryList {
		messagePath := filepath.Join(md.newDir, message)
		messageData, err := os.ReadFile(messagePath)
		if err != nil {
			return nil, err
		}

		tmpMessage := &Message{}

		err = json.Unmarshal(messageData, tmpMessage)
		if err != nil {
			log.Printf("error unmarshaling: %v", err)
			continue
		}

		messageList = append(messageList, *tmpMessage)
		os.Remove(messagePath)
	}

	return messageList, nil
}

package work

import (
	"encoding/json"
	"fmt"

	"github.com/joshstrohminger/TorrentProcessor/internal/torrent"
)

type JsonFormat struct {
}

func (j JsonFormat) Ext() string {
	return ".json"
}

func (j JsonFormat) Unmarshal(data []byte) (*torrent.Entry, error) {
	entry := new(torrent.Entry)
	if err := json.Unmarshal(data, entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}
	return entry, nil
}

func (j JsonFormat) Marshal(entry torrent.Entry) ([]byte, error) {
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return data, nil
}

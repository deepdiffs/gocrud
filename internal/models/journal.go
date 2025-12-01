package models

// JournalEntry represents a parsed journal entry with structured fields.
type JournalEntry struct {
	Title          string   `json:"title"`
	Date           string   `json:"date"`
	Emotions       []string `json:"emotions"`
	People         []string `json:"people"`
	Topics         []string `json:"topics"`
	Content        string   `json:"content"`
	ImageData      string   `json:"image_data,omitempty"`
	ImageFormat    string   `json:"image_format,omitempty"`
	ImageSizeBytes int64    `json:"image_size_bytes,omitempty"`
}

// JournalRequest represents the incoming payload for journal entries.
type JournalRequest struct {
	Entry    JournalEntry `json:"entry"`
	FileName string       `json:"fileName"`
}

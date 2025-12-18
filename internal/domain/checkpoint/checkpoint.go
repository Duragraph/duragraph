package checkpoint

import (
	"time"

	"github.com/google/uuid"
)

// Checkpoint represents a state snapshot of a thread at a point in time
type Checkpoint struct {
	id                 string
	threadID           string
	checkpointNS       string
	checkpointID       string
	parentCheckpointID string
	channelValues      map[string]interface{}
	channelVersions    map[string]int
	versionsSeen       map[string]map[string]int
	pendingSends       []map[string]interface{}
	createdAt          time.Time
}

// NewCheckpoint creates a new checkpoint
func NewCheckpoint(
	threadID string,
	checkpointNS string,
	checkpointID string,
	parentCheckpointID string,
	channelValues map[string]interface{},
) (*Checkpoint, error) {
	if threadID == "" {
		return nil, ErrInvalidThreadID
	}
	if checkpointID == "" {
		checkpointID = uuid.New().String()
	}

	return &Checkpoint{
		id:                 uuid.New().String(),
		threadID:           threadID,
		checkpointNS:       checkpointNS,
		checkpointID:       checkpointID,
		parentCheckpointID: parentCheckpointID,
		channelValues:      channelValues,
		channelVersions:    make(map[string]int),
		versionsSeen:       make(map[string]map[string]int),
		pendingSends:       make([]map[string]interface{}, 0),
		createdAt:          time.Now(),
	}, nil
}

// Reconstitute creates a checkpoint from persisted data
func Reconstitute(
	id string,
	threadID string,
	checkpointNS string,
	checkpointID string,
	parentCheckpointID string,
	channelValues map[string]interface{},
	channelVersions map[string]int,
	versionsSeen map[string]map[string]int,
	pendingSends []map[string]interface{},
	createdAt time.Time,
) *Checkpoint {
	return &Checkpoint{
		id:                 id,
		threadID:           threadID,
		checkpointNS:       checkpointNS,
		checkpointID:       checkpointID,
		parentCheckpointID: parentCheckpointID,
		channelValues:      channelValues,
		channelVersions:    channelVersions,
		versionsSeen:       versionsSeen,
		pendingSends:       pendingSends,
		createdAt:          createdAt,
	}
}

// Getters
func (c *Checkpoint) ID() string                              { return c.id }
func (c *Checkpoint) ThreadID() string                        { return c.threadID }
func (c *Checkpoint) CheckpointNS() string                    { return c.checkpointNS }
func (c *Checkpoint) CheckpointID() string                    { return c.checkpointID }
func (c *Checkpoint) ParentCheckpointID() string              { return c.parentCheckpointID }
func (c *Checkpoint) ChannelValues() map[string]interface{}   { return c.channelValues }
func (c *Checkpoint) ChannelVersions() map[string]int         { return c.channelVersions }
func (c *Checkpoint) VersionsSeen() map[string]map[string]int { return c.versionsSeen }
func (c *Checkpoint) PendingSends() []map[string]interface{}  { return c.pendingSends }
func (c *Checkpoint) CreatedAt() time.Time                    { return c.createdAt }

// SetChannelValue sets a value for a channel
func (c *Checkpoint) SetChannelValue(channel string, value interface{}) {
	c.channelValues[channel] = value
	c.channelVersions[channel]++
}

// GetChannelValue gets a value for a channel
func (c *Checkpoint) GetChannelValue(channel string) (interface{}, bool) {
	val, ok := c.channelValues[channel]
	return val, ok
}

// AddPendingSend adds a pending send to the checkpoint
func (c *Checkpoint) AddPendingSend(send map[string]interface{}) {
	c.pendingSends = append(c.pendingSends, send)
}

// ClearPendingSends clears all pending sends
func (c *Checkpoint) ClearPendingSends() {
	c.pendingSends = make([]map[string]interface{}, 0)
}

// CheckpointWrite represents a single channel write within a checkpoint
type CheckpointWrite struct {
	id           string
	threadID     string
	checkpointNS string
	checkpointID string
	taskID       string
	idx          int
	channel      string
	writeType    string
	blob         map[string]interface{}
	createdAt    time.Time
}

// NewCheckpointWrite creates a new checkpoint write
func NewCheckpointWrite(
	threadID string,
	checkpointNS string,
	checkpointID string,
	taskID string,
	idx int,
	channel string,
	writeType string,
	blob map[string]interface{},
) *CheckpointWrite {
	return &CheckpointWrite{
		id:           uuid.New().String(),
		threadID:     threadID,
		checkpointNS: checkpointNS,
		checkpointID: checkpointID,
		taskID:       taskID,
		idx:          idx,
		channel:      channel,
		writeType:    writeType,
		blob:         blob,
		createdAt:    time.Now(),
	}
}

// Getters
func (w *CheckpointWrite) ID() string                   { return w.id }
func (w *CheckpointWrite) ThreadID() string             { return w.threadID }
func (w *CheckpointWrite) CheckpointNS() string         { return w.checkpointNS }
func (w *CheckpointWrite) CheckpointID() string         { return w.checkpointID }
func (w *CheckpointWrite) TaskID() string               { return w.taskID }
func (w *CheckpointWrite) Idx() int                     { return w.idx }
func (w *CheckpointWrite) Channel() string              { return w.channel }
func (w *CheckpointWrite) WriteType() string            { return w.writeType }
func (w *CheckpointWrite) Blob() map[string]interface{} { return w.blob }
func (w *CheckpointWrite) CreatedAt() time.Time         { return w.createdAt }

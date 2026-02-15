package ingest

import (
	"encoding/json"
	"time"

	"github.com/snarg/tr-engine/internal/database"
)

func (p *Pipeline) handleTrunkingMessage(topic string, payload []byte) error {
	var msg TrunkingMessageMsg
	if err := json.Unmarshal(payload, &msg); err != nil {
		return err
	}

	ts := time.Unix(msg.Timestamp, 0)
	data := msg.Message

	var systemID *int
	if sid := p.identity.GetSystemIDForSysName(data.SysName); sid != 0 {
		systemID = &sid
	}

	// Convert meta to jsonb-compatible bytes; empty string → null
	var meta []byte
	if data.Meta != "" {
		meta = []byte(`"` + data.Meta + `"`)
	}

	row := database.TrunkingMessageRow{
		SystemID:     systemID,
		SysNum:       int16(data.SysNum),
		SysName:      data.SysName,
		TrunkMsg:     data.TrunkMsg,
		TrunkMsgType: data.TrunkMsgType,
		Opcode:       data.Opcode,
		OpcodeType:   data.OpcodeType,
		OpcodeDesc:   data.OpcodeDesc,
		Meta:         meta,
		Time:         ts,
		InstanceID:   msg.InstanceID,
	}
	p.trunkingBatcher.Add(row)

	sysID := 0
	if systemID != nil {
		sysID = *systemID
	}
	p.PublishEvent(EventData{
		Type:     "trunking_message",
		SystemID: sysID,
		Payload: map[string]any{
			"system_id":      sysID,
			"sys_name":       data.SysName,
			"trunk_msg":      data.TrunkMsg,
			"trunk_msg_type": data.TrunkMsgType,
			"opcode":         data.Opcode,
			"opcode_type":    data.OpcodeType,
			"opcode_desc":    data.OpcodeDesc,
			"time":           ts,
		},
	})

	return nil
}
